"""Opt-in disposable database provisioning test; touches no shared database."""
from pathlib import Path
import json
import os
import secrets
import subprocess
import tempfile
import time
import unittest

ROOT=Path(__file__).resolve().parents[2]
IMAGE='postgres@sha256:e62fbf9d3e2b49816a32c400ed2dba83e3b361e6833e624024309c35d334b412'


class Provisioning(unittest.TestCase):
    def test_idempotent_nonprivileged_role_and_credential_file_policy(self):
        name='n8n-qualification-role-'+secrets.token_hex(5)
        with tempfile.TemporaryDirectory() as directory:
            admin=Path(directory)/'postgres.env';consumer=Path(directory)/'n8n.env'
            for path,content in [(admin,'POSTGRES_USER=fixture\nPOSTGRES_DB=fixture\nPOSTGRES_PASSWORD='+secrets.token_hex(20)+'\n'),
                                 (consumer,'N8N_DB_PASSWORD='+secrets.token_hex(20)+'\n')]:
                path.write_text(content);path.chmod(0o600)
            try:
                subprocess.run(['docker','run','-d','--name',name,'--network','none','--memory','256m',
                    '--label','co.prls.fixture='+name,'--tmpfs','/var/lib/postgresql/data:rw,size=256m',
                    '--env-file',str(admin),IMAGE],check=True,capture_output=True)
                for _ in range(40):
                    ready=subprocess.run(['docker','exec',name,'pg_isready','-h','127.0.0.1','-U','fixture','-d','fixture'],capture_output=True)
                    if ready.returncode==0:break
                    time.sleep(0.5)
                self.assertEqual(ready.returncode,0)
                cmd=['python3',str(ROOT/'scripts/provision-n8n-postgres.py'),'--fixture','--container',name,'--env-file',str(consumer)]
                for _ in range(2):
                    result=subprocess.run(cmd,text=True,capture_output=True)
                    self.assertEqual(result.returncode,0,'Provisioning failed; secret-bearing output suppressed')
                    data=json.loads(result.stdout)
                    self.assertTrue(data['loginVerified']);self.assertFalse(data['elevatedRole'])
                consumer.chmod(0o644)
                result=subprocess.run(cmd,text=True,capture_output=True)
                self.assertNotEqual(result.returncode,0)
                self.assertIn('mode 0600',result.stderr)
            finally:
                subprocess.run(['docker','rm','-f',name],capture_output=True)


if __name__=='__main__':unittest.main()
