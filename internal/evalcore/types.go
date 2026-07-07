package evalcore

import "fmt"

const (
	AnswerYes = "yes"
	AnswerNo  = "no"
)

type ErrorCode string

const (
	CodeInvalidQuestionGeneration ErrorCode = "invalid_question_generation"
	CodeInvalidWeights            ErrorCode = "invalid_weights"
	CodeInvalidJudgments          ErrorCode = "invalid_judgments"
)

type SemanticError struct {
	Code    ErrorCode
	Message string
}

func (e *SemanticError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type DraftQuestion struct {
	Rationale string `json:"rationale"`
	Question  string `json:"question"`
}

type CandidateQuestion struct {
	ID        string `json:"id"`
	Ordinal   int    `json:"ordinal"`
	Rationale string `json:"rationale"`
	Question  string `json:"question"`
}

type Weight struct {
	QuestionID string `json:"question_id"`
	Rationale  string `json:"rationale"`
	Weight     int    `json:"weight"`
}

type ActiveQuestion struct {
	ID       string `json:"id"`
	Ordinal  int    `json:"ordinal"`
	Question string `json:"question"`
	Weight   int    `json:"weight"`
}

type Judgment struct {
	QuestionID string `json:"question_id"`
	Evidence   string `json:"evidence"`
	Answer     string `json:"answer"`
}

type ScoreResult struct {
	SatisfiedPoints     int      `json:"satisfied_points"`
	TotalPossiblePoints int      `json:"total_possible_points"`
	ChecklistPassRate   float64  `json:"checklist_pass_rate"`
	FailedQuestionIDs   []string `json:"failed_question_ids"`
}

func semanticError(code ErrorCode, format string, args ...any) *SemanticError {
	return &SemanticError{Code: code, Message: fmt.Sprintf(format, args...)}
}
