package protocol

const RequiredCodexCLIVersion = "0.138.0"

const (
	MethodInitialize = "initialize"
	MethodModelList = "model/list"
	MethodExperimentalFeatureList = "experimentalFeature/list"
	MethodMCPServerStatusList = "mcpServerStatus/list"
	MethodThreadStart = "thread/start"
	MethodTurnStart = "turn/start"
	MethodTurnInterrupt = "turn/interrupt"
	MethodTurnSteer = "turn/steer"
	MethodThreadRead = "thread/read"
	MethodThreadTurnsList = "thread/turns/list"
	MethodThreadCompactStart = "thread/compact/start"
	MethodThreadGoalSet = "thread/goal/set"
	MethodThreadGoalGet = "thread/goal/get"
	MethodThreadGoalClear = "thread/goal/clear"
	MethodReviewStart = "review/start"
)
