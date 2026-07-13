package evalcore

import "fmt"

const (
	AnswerYes = "yes"
	AnswerNo  = "no"
)

const (
	DefaultMaxDimensions             = 6
	DefaultMaxCandidatesPerDimension = 8
	DefaultMaxSplitCount             = 4
	DefaultMaxFinalQuestions         = 64
	DefaultEvaluationRuns            = 3
	DefaultMaxEvaluationRuns         = 5
)

type ErrorCode string

const (
	CodeInvalidQuestionGeneration ErrorCode = "invalid_question_generation"
	CodeInvalidDimensionAnalysis  ErrorCode = "invalid_dimension_analysis"
	CodeInvalidWeights            ErrorCode = "invalid_weights"
	CodeInvalidSplits             ErrorCode = "invalid_splits"
	CodeInvalidFinalChecklist     ErrorCode = "invalid_final_checklist"
	CodeInvalidJudgments          ErrorCode = "invalid_judgments"
)

type SemanticError struct {
	Code        ErrorCode
	Message     string
	Diagnostics []LimitDiagnostic
}

func (e *SemanticError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type ChecklistLimits struct {
	MaxDimensions             int `json:"max_dimensions"`
	MaxCandidatesPerDimension int `json:"max_candidates_per_dimension"`
	MaxSplitCount             int `json:"max_split_count"`
	MaxFinalQuestions         int `json:"max_final_questions"`
}

func DefaultChecklistLimits() ChecklistLimits {
	return ChecklistLimits{
		MaxDimensions:             DefaultMaxDimensions,
		MaxCandidatesPerDimension: DefaultMaxCandidatesPerDimension,
		MaxSplitCount:             DefaultMaxSplitCount,
		MaxFinalQuestions:         DefaultMaxFinalQuestions,
	}
}

func (l ChecklistLimits) WithDefaults() ChecklistLimits {
	defaults := DefaultChecklistLimits()
	if l.MaxDimensions <= 0 {
		l.MaxDimensions = defaults.MaxDimensions
	}
	if l.MaxCandidatesPerDimension <= 0 {
		l.MaxCandidatesPerDimension = defaults.MaxCandidatesPerDimension
	}
	if l.MaxSplitCount <= 0 {
		l.MaxSplitCount = defaults.MaxSplitCount
	}
	if l.MaxFinalQuestions <= 0 {
		l.MaxFinalQuestions = defaults.MaxFinalQuestions
	}
	return l
}

func (l ChecklistLimits) Validate() error {
	l = l.WithDefaults()
	if l.MaxSplitCount > DefaultMaxSplitCount {
		return fmt.Errorf("max_split_count cannot exceed %d", DefaultMaxSplitCount)
	}
	return nil
}

type LimitDiagnostic struct {
	LimitName       string `json:"limit_name"`
	ConfiguredLimit int    `json:"configured_limit"`
	ObservedCount   int    `json:"observed_count"`
	ChecklistID     string `json:"checklist_id,omitempty"`
	Stage           string `json:"stage,omitempty"`
}

type DraftDimension struct {
	Name      string `json:"name"`
	Rubric    string `json:"rubric"`
	Rationale string `json:"rationale"`
}

type Dimension struct {
	ID        string `json:"id"`
	Ordinal   int    `json:"ordinal"`
	Name      string `json:"name"`
	Rubric    string `json:"rubric"`
	Rationale string `json:"rationale"`
}

type DraftQuestion struct {
	Rationale string `json:"rationale"`
	Question  string `json:"question"`
}

type CandidateQuestion struct {
	ID          string `json:"id"`
	DimensionID string `json:"dimension_id"`
	Ordinal     int    `json:"ordinal"`
	Rationale   string `json:"rationale"`
	Question    string `json:"question"`
}

type Weight struct {
	CandidateQuestionID string `json:"candidate_question_id"`
	Rationale           string `json:"rationale"`
	Weight              int    `json:"weight"`
}

type SplitQuestions struct {
	CandidateQuestionID string          `json:"candidate_question_id"`
	Questions           []DraftQuestion `json:"questions"`
}

type FinalQuestion struct {
	ID                string `json:"id"`
	Ordinal           int    `json:"ordinal"`
	DimensionID       string `json:"dimension_id"`
	SourceCandidateID string `json:"source_candidate_id"`
	Rationale         string `json:"rationale"`
	Question          string `json:"question"`
}

type Judgment struct {
	QuestionID string `json:"question_id"`
	Evidence   string `json:"evidence"`
	Answer     string `json:"answer"`
}

type RunJudgment struct {
	RunIndex   int    `json:"run_index"`
	QuestionID string `json:"question_id"`
	Evidence   string `json:"evidence"`
	Answer     string `json:"answer"`
}

type JudgmentRun struct {
	RunIndex int    `json:"run_index"`
	Evidence string `json:"evidence"`
	Answer   string `json:"answer"`
}

type AggregatedJudgment struct {
	QuestionID string        `json:"question_id"`
	Runs       []JudgmentRun `json:"runs"`
	Answer     string        `json:"answer"`
}

type ScoreResult struct {
	SatisfiedPoints     int      `json:"satisfied_points"`
	TotalPossiblePoints int      `json:"total_possible_points"`
	ChecklistPassRate   float64  `json:"checklist_pass_rate"`
	FailedQuestionIDs   []string `json:"failed_question_ids"`
}

type AggregationResult struct {
	Judgments []AggregatedJudgment
	Score     ScoreResult
}

func ValidateEvaluationRuns(evaluationRuns, maxEvaluationRuns int) error {
	if maxEvaluationRuns <= 0 {
		maxEvaluationRuns = DefaultMaxEvaluationRuns
	}
	if evaluationRuns <= 0 || evaluationRuns%2 == 0 || evaluationRuns > maxEvaluationRuns {
		return fmt.Errorf("evaluation_runs must be an odd positive integer not greater than %d", maxEvaluationRuns)
	}
	return nil
}

func semanticError(code ErrorCode, format string, args ...any) *SemanticError {
	return &SemanticError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func limitError(code ErrorCode, diagnostic LimitDiagnostic) *SemanticError {
	return &SemanticError{
		Code: code,
		Message: fmt.Sprintf("%s exceeded: configured_limit=%d observed_count=%d checklist_id=%s stage=%s",
			diagnostic.LimitName,
			diagnostic.ConfiguredLimit,
			diagnostic.ObservedCount,
			diagnostic.ChecklistID,
			diagnostic.Stage,
		),
		Diagnostics: []LimitDiagnostic{diagnostic},
	}
}
