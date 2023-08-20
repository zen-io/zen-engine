package engine

// special error that signals to stop the execution without errors
type DoNotContinue struct{}

func (dnc DoNotContinue) Error() string {
	return "do not continue"
}

// special error that signals the current target  doesnt support the passed script
type ScriptNotSupported struct{}

func (dnc ScriptNotSupported) Error() string {
	return "script not supported"
}