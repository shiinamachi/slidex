package protocol

const RequiredCodexCLIVersion = "0.132.0"

const (
	MethodInitialize              = "initialize"
	MethodModelList               = "model/list"
	MethodExperimentalFeatureList = "experimentalFeature/list"
	MethodMCPServerStatusList     = "mcpServerStatus/list"
	MethodThreadStart             = "thread/start"
	MethodTurnStart               = "turn/start"
	MethodThreadGoalSet           = "thread/goal/set"
	MethodReviewStart             = "review/start"
)
