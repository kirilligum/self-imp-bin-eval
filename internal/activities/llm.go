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
	ActivityWriteChecklistInputs          = "WriteChecklistInputs"
	ActivityWriteEvaluationInput          = "WriteEvaluationInput"
	ActivityAnalyzeDimensions             = "AnalyzeDimensions"
	ActivityGenerateQuestionsForDimension = "GenerateQuestionsForDimension"
	ActivityAssignWeights                 = "AssignWeights"
	ActivitySplitQuestion                 = "SplitQuestion"
	ActivityJudgeAnswer                   = "JudgeAnswer"
	ActivityLoadChecklist                 = "LoadChecklist"
	ActivitySucceedChecklist              = "SucceedChecklist"
	ActivityFailChecklist                 = "FailChecklist"
	ActivitySucceedEvaluation             = "SucceedEvaluation"
	ActivityFailEvaluation                = "FailEvaluation"
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

type AnalyzeDimensionsInput struct {
	ChecklistID string
	Task        string
	Context     string
	Limits      evalcore.ChecklistLimits
}

type AnalyzeDimensionsResult struct {
	Dimensions []evalcore.Dimension
}

type GenerateQuestionsForDimensionInput struct {
	ChecklistID string
	Task        string
	Context     string
	Dimension   evalcore.Dimension
	Limits      evalcore.ChecklistLimits
}

type GenerateQuestionsForDimensionResult struct {
	Questions []evalcore.DraftQuestion
}

type AssignWeightsInput struct {
	ChecklistID        string
	Task               string
	Context            string
	CandidateQuestions []evalcore.CandidateQuestion
	Limits             evalcore.ChecklistLimits
}

type AssignWeightsResult struct {
	Weights []evalcore.Weight
}

type SplitQuestionInput struct {
	ChecklistID       string
	Task              string
	Context           string
	CandidateQuestion evalcore.CandidateQuestion
	Weight            evalcore.Weight
	Limits            evalcore.ChecklistLimits
}

type SplitQuestionResult struct {
	Split evalcore.SplitQuestions
}

type JudgeAnswerInput struct {
	EvaluationID string
	Task         string
	Context      string
	ModelAnswer  string
	Questions    []evalcore.FinalQuestion
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
	ChecklistID        string
	Dimensions         []evalcore.Dimension
	CandidateQuestions []evalcore.CandidateQuestion
	Weights            []evalcore.Weight
	Questions          []evalcore.FinalQuestion
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

func (a *Activities) AnalyzeDimensions(ctx context.Context, in AnalyzeDimensionsInput) (AnalyzeDimensionsResult, error) {
	limits := in.Limits.WithDefaults()
	req := llm.BuildDimensionAnalysisRequest(in.Task, in.Context, a.modelProfile, limits)
	var out llm.DimensionAnalysisOutput
	if err := a.runChecklistLLM(ctx, in.ChecklistID, artifacts.PromptDimensionAnalysis, req, &out); err != nil {
		return AnalyzeDimensionsResult{}, err
	}
	if err := evalcore.ValidateDimensionGeneration(out.Dimensions, limits); err != nil {
		return AnalyzeDimensionsResult{}, ToTemporalError(err)
	}
	dimensions := evalcore.AssignDimensionIDs(out.Dimensions)
	if err := evalcore.ValidateDimensions(dimensions, limits); err != nil {
		return AnalyzeDimensionsResult{}, ToTemporalError(err)
	}
	return AnalyzeDimensionsResult{Dimensions: dimensions}, nil
}

func (a *Activities) GenerateQuestionsForDimension(ctx context.Context, in GenerateQuestionsForDimensionInput) (GenerateQuestionsForDimensionResult, error) {
	limits := in.Limits.WithDefaults()
	req := llm.BuildQuestionGenerationRequest(in.Task, in.Context, a.modelProfile, in.Dimension, limits)
	var out llm.QuestionGenerationOutput
	if err := a.runChecklistLLM(ctx, in.ChecklistID, artifacts.PromptQuestionGeneration+"/"+in.Dimension.ID, req, &out); err != nil {
		return GenerateQuestionsForDimensionResult{}, err
	}
	if err := evalcore.ValidateQuestionGeneration(out.Questions, limits); err != nil {
		return GenerateQuestionsForDimensionResult{}, ToTemporalError(err)
	}
	return GenerateQuestionsForDimensionResult{Questions: out.Questions}, nil
}

func (a *Activities) AssignWeights(ctx context.Context, in AssignWeightsInput) (AssignWeightsResult, error) {
	limits := in.Limits.WithDefaults()
	req := llm.BuildWeightAssignmentRequest(in.Task, in.Context, a.modelProfile, in.CandidateQuestions, limits)
	var out llm.WeightAssignmentOutput
	if err := a.runChecklistLLM(ctx, in.ChecklistID, artifacts.PromptWeightAssignment, req, &out); err != nil {
		return AssignWeightsResult{}, err
	}
	if err := evalcore.ValidateWeights(in.CandidateQuestions, out.Weights, limits); err != nil {
		return AssignWeightsResult{}, ToTemporalError(err)
	}
	return AssignWeightsResult{Weights: out.Weights}, nil
}

func (a *Activities) SplitQuestion(ctx context.Context, in SplitQuestionInput) (SplitQuestionResult, error) {
	limits := in.Limits.WithDefaults()
	req := llm.BuildQuestionSplittingRequest(in.Task, in.Context, a.modelProfile, in.CandidateQuestion, in.Weight, limits)
	var out llm.QuestionSplittingOutput
	if err := a.runChecklistLLM(ctx, in.ChecklistID, artifacts.PromptQuestionSplitting+"/"+in.CandidateQuestion.ID, req, &out); err != nil {
		return SplitQuestionResult{}, err
	}
	split := evalcore.SplitQuestions{CandidateQuestionID: in.CandidateQuestion.ID, Questions: out.Questions}
	if err := evalcore.ValidateSplitQuestions(split, in.Weight.Weight); err != nil {
		return SplitQuestionResult{}, ToTemporalError(err)
	}
	return SplitQuestionResult{Split: split}, nil
}

func (a *Activities) JudgeAnswer(ctx context.Context, in JudgeAnswerInput) (JudgeAnswerResult, error) {
	req := llm.BuildBinaryJudgingRequest(in.Task, in.Context, in.ModelAnswer, a.modelProfile, in.Questions)
	var out llm.BinaryJudgingOutput
	if err := a.runEvaluationLLM(ctx, in.EvaluationID, artifacts.PromptBinaryJudging, req, &out); err != nil {
		return JudgeAnswerResult{}, err
	}
	if err := evalcore.ValidateJudgments(in.Questions, out.Judgments); err != nil {
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
