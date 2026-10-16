#!/usr/bin/env python3
"""Provision an isolated logical n8n database on the existing shared daemon.

Reads N8N_DB_PASSWORD from a mode-0600 owner-consumer env file. It never
prints credentials, changes an existing password, starts PostgreSQL or deletes data.
"""
import argparse
import json
from pathlib import Path
import shlex
import subprocess


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--env-file', type=Path, required=True)
    parser.add_argument('--container', default='compose-postgres-1')
    parser.add_argument('--database', choices=['n8n', 'aik_n8n'], default='n8n')
    parser.add_argument('--fixture', action='store_true')
    args = parser.parse_args()
    if args.env_file.stat().st_mode & 0o077:
        raise SystemExit('Credential input must be mode 0600')
    values = {}
    for line in args.env_file.read_text().splitlines():
        if '=' in line and not line.lstrip().startswith('#'):
            key, value = line.split('=', 1)
            words = shlex.split(value, comments=True)
            values[key.strip()] = words[0] if words else ''
    password = values.get('N8N_DB_PASSWORD')
    if not password or '\n' in password:
        raise SystemExit('Missing or invalid consumer database password')
    container = json.loads(subprocess.check_output(['docker', 'inspect', args.container], text=True))[0]
    labels = container['Config'].get('Labels', {})
    if args.fixture:
        if not labels.get('co.prls.fixture', '').startswith('n8n-qualification-'):
            raise SystemExit('Fixture flag requires an explicitly labeled disposable PostgreSQL')
    else:
        volume_names = {m.get('Name') for m in container['Mounts']}
        if (labels.get('com.docker.compose.project'), labels.get('com.docker.compose.service')) != ('compose', 'postgres') or 'compose_postgres-data' not in volume_names:
            raise SystemExit('Shared PostgreSQL owner/volume identity mismatch')
    database = args.database
    literal = "'" + password.replace("'", "''") + "'"
    sql = f"""SELECT format('CREATE ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD %L', '{database}', {literal})
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '{database}')\gexec
SELECT format('CREATE DATABASE %I OWNER %I', '{database}', '{database}')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '{database}')\gexec
REVOKE ALL ON DATABASE {database} FROM PUBLIC;
ALTER DATABASE {database} CONNECTION LIMIT 20;
"""
    result = subprocess.run(['docker', 'exec', '-i', args.container, 'sh', '-c',
        'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -X -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1'],
        input=sql, text=True, capture_output=True)
    if result.returncode:
        raise SystemExit('Provisioning failed; no credential-bearing SQL is emitted')
    # Verify the existing or created password and prohibit elevated role flags.
    check = subprocess.run(['docker', 'exec', '-i', args.container, 'sh', '-c',
        'read -r PGPASSWORD; export PGPASSWORD; exec psql -X -h 127.0.0.1 -U '+database+' -d '+database+
        ' -At -v ON_ERROR_STOP=1 -c "SELECT rolname,rolsuper,rolcreatedb,rolcreaterole FROM pg_roles WHERE rolname=current_user"'],
        input=password+'\n', text=True, capture_output=True)
    if check.returncode or check.stdout.strip() != database+'|f|f|f':
        raise SystemExit('Existing credential or role verification failed; password was not rotated')
    print(json.dumps({'database': database, 'role': database, 'container': args.container,
                      'loginVerified': True, 'elevatedRole': False, 'connectionLimit': 20}))


if __name__ == '__main__':
    main()
