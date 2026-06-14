package installer

import "fmt"

// StepError is the actionable error produced when a step fails. It carries
// everything the user needs to recover: what failed, why the step mattered,
// whether retry is safe, the next command, and the log path. Bare "install
// failed" messages are never surfaced.
type StepError struct {
	Step      string // the step name that failed
	Cause     error  // the underlying failure
	Impact    string // why the step matters / what is affected
	SafeRetry bool   // whether re-running is safe (steps are idempotent)
	Next      string // the exact command to run next
	LogPath   string // where the full log lives
}

func (e *StepError) Error() string {
	retry := "no"
	if e.SafeRetry {
		retry = "yes — steps are idempotent and resume from here"
	}
	return fmt.Sprintf(
		"step %q failed: %v\n"+
			"  impact:        %s\n"+
			"  safe to retry: %s\n"+
			"  next:          %s\n"+
			"  log:           %s",
		e.Step, e.Cause, e.Impact, retry, e.Next, e.LogPath)
}

func (e *StepError) Unwrap() error { return e.Cause }
