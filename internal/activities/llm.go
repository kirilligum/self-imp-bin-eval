package activities

import (
	"context"
	"encoding/json"

	"github.com/kirilligum/self-imp-bin-eval/internal/artifacts"
	"github.com/kirilligum/self-imp-bin-eval/internal/db"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/kirilligum/self-imp-bin-eval/internal/llm"
)

const (
	ActivityWriteChecklistInputs = "WriteChecklistInputs"
	ActivityWriteEvaluationInput = "WriteEvaluationInput"
	ActivityGenerateQuestions    = "GenerateQuestions"
	ActivityAssignWeights        = "AssignWeights"
	ActivityJudgeAnswer          = "JudgeAnswer"
	ActivityLoadChecklist        = "LoadChecklist"
	ActivitySucceedChecklist     = "SucceedChecklist"
	ActivityFailChecklist        = "FailChecklist"
	ActivitySucceedEvaluation    = "SucceedEvaluation"
	ActivityFailEvaluation       = "FailEvaluation"
)

type Dependencies struct {
	Artifacts    artifacts.Writer
	LLM          llm.LLMClient
	Store        *db.Store
	ModelProfile string
}

type Activities struct {
	artifacts    artifacts.Writer
	llm          llm.LLMClient
	store        *db.Store
	modelProfile string
}

func New(deps Dependencies) *Activities {
	return &Activities{
		artifacts:    deps.Artifacts,
		llm:          deps.LLM,
		store:        deps.Store,
		modelProfile: deps.ModelProfile,
	}
}

type WriteChecklistInputsInput struct {
	ChecklistID string
	Task        string
	Context     string
}

type WriteEvaluationInputInput struct {
	EvaluationID string
	ModelAnswer  string
}

type GenerateQuestionsInput struct {
	ChecklistID string
	Task        string
	Context     string
}

type GenerateQuestionsResult struct {
	Questions []evalcore.CandidateQuestion
}

type AssignWeightsInput struct {
	ChecklistID string
	Task        string
	Context     string
	Questions   []evalcore.CandidateQuestion
}

type AssignWeightsResult struct {
	Weights []evalcore.Weight
}

type JudgeAnswerInput struct {
	EvaluationID string
	Task         string
	Context      string
	ModelAnswer  string
	Questions    []evalcore.CandidateQuestion
	Weights      []evalcore.Weight
}

type JudgeAnswerResult struct {
	Judgments []evalcore.Judgment
}

type LoadChecklistInput struct {
	ChecklistID string
}

type LoadChecklistResult struct {
	Checklist db.Checklist
	Task      string
	Context   string
}

type SucceedChecklistInput struct {
	ChecklistID string
	Questions   []evalcore.CandidateQuestion
	Weights     []evalcore.Weight
}

type FailChecklistInput struct {
	ChecklistID  string
	ErrorMessage string
}

type SucceedEvaluationInput struct {
	EvaluationID string
	ChecklistID  string
	Judgments    []evalcore.Judgment
	Score        evalcore.ScoreResult
}

type FailEvaluationInput struct {
	EvaluationID string
	ChecklistID  string
	ErrorMessage string
}

func (a *Activities) WriteChecklistInputs(ctx context.Context, in WriteChecklistInputsInput) error {
	if err := a.artifacts.Write(ctx, artifacts.ChecklistTaskKey(in.ChecklistID), []byte(in.Task)); err != nil {
		return ToTemporalError(err)
	}
	if err := a.artifacts.Write(ctx, artifacts.ChecklistContextKey(in.ChecklistID), []byte(in.Context)); err != nil {
		return ToTemporalError(err)
	}
	return nil
}

func (a *Activities) WriteEvaluationInput(ctx context.Context, in WriteEvaluationInputInput) error {
	if err := a.artifacts.Write(ctx, artifacts.EvaluationAnswerKey(in.EvaluationID), []byte(in.ModelAnswer)); err != nil {
		return ToTemporalError(err)
	}
	return nil
}

func (a *Activities) GenerateQuestions(ctx context.Context, in GenerateQuestionsInput) (GenerateQuestionsResult, error) {
	req := llm.BuildQuestionGenerationRequest(in.Task, in.Context, a.modelProfile)
	var out llm.QuestionGenerationOutput
	if err := a.runChecklistLLM(ctx, in.ChecklistID, artifacts.PromptQuestionGeneration, req, &out); err != nil {
		return GenerateQuestionsResult{}, err
	}
	if err := evalcore.ValidateQuestionGeneration(out.Questions); err != nil {
		return GenerateQuestionsResult{}, ToTemporalError(err)
	}
	return GenerateQuestionsResult{Questions: evalcore.AssignQuestionIDs(out.Questions)}, nil
}

func (a *Activities) AssignWeights(ctx context.Context, in AssignWeightsInput) (AssignWeightsResult, error) {
	req := llm.BuildWeightAssignmentRequest(in.Task, in.Context, a.modelProfile, in.Questions)
	var out llm.WeightAssignmentOutput
	if err := a.runChecklistLLM(ctx, in.ChecklistID, artifacts.PromptWeightAssignment, req, &out); err != nil {
		return AssignWeightsResult{}, err
	}
	if err := evalcore.ValidateWeights(in.Questions, out.Weights); err != nil {
		return AssignWeightsResult{}, ToTemporalError(err)
	}
	return AssignWeightsResult{Weights: out.Weights}, nil
}

func (a *Activities) JudgeAnswer(ctx context.Context, in JudgeAnswerInput) (JudgeAnswerResult, error) {
	active, err := evalcore.BuildActiveChecklist(in.Questions, in.Weights)
	if err != nil {
		return JudgeAnswerResult{}, ToTemporalError(err)
	}
	req := llm.BuildBinaryJudgingRequest(in.Task, in.Context, in.ModelAnswer, a.modelProfile, active)
	var out llm.BinaryJudgingOutput
	if err := a.runEvaluationLLM(ctx, in.EvaluationID, artifacts.PromptBinaryJudging, req, &out); err != nil {
		return JudgeAnswerResult{}, err
	}
	if err := evalcore.ValidateJudgments(in.Questions, in.Weights, out.Judgments); err != nil {
		return JudgeAnswerResult{}, ToTemporalError(err)
	}
	return JudgeAnswerResult{Judgments: out.Judgments}, nil
}

func (a *Activities) runChecklistLLM(ctx context.Context, checklistID, prompt string, req llm.GenerateRequest, out any) error {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return ToTemporalError(err)
	}
	if err := a.artifacts.Write(ctx, artifacts.ChecklistLLMRequestKey(checklistID, prompt), reqBytes); err != nil {
		return ToTemporalError(err)
	}
	if err := a.llm.GenerateJSON(ctx, req, out); err != nil {
		return ToTemporalError(err)
	}
	respBytes, err := json.Marshal(out)
	if err != nil {
		return ToTemporalError(err)
	}
	if err := a.artifacts.Write(ctx, artifacts.ChecklistLLMResponseKey(checklistID, prompt), respBytes); err != nil {
		return ToTemporalError(err)
	}
	return nil
}

func (a *Activities) runEvaluationLLM(ctx context.Context, evaluationID, prompt string, req llm.GenerateRequest, out any) error {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return ToTemporalError(err)
	}
	if err := a.artifacts.Write(ctx, artifacts.EvaluationLLMRequestKey(evaluationID, prompt), reqBytes); err != nil {
		return ToTemporalError(err)
	}
	if err := a.llm.GenerateJSON(ctx, req, out); err != nil {
		return ToTemporalError(err)
	}
	respBytes, err := json.Marshal(out)
	if err != nil {
		return ToTemporalError(err)
	}
	if err := a.artifacts.Write(ctx, artifacts.EvaluationLLMResponseKey(evaluationID, prompt), respBytes); err != nil {
		return ToTemporalError(err)
	}
	return nil
}
