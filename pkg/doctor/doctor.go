// Package doctor runs read-only integrity checks against a remindb store and applies opt-in fixes.
package doctor

import (
	"context"
	"fmt"

	"github.com/radimsem/remindb/pkg/store"
)

type Status int

const (
	Pass Status = iota
	Warn
	Fail
)

func (s Status) String() string {
	switch s {
	case Pass:
		return "pass"
	case Warn:
		return "warn"
	case Fail:
		return "fail"
	default:
		return "unknown"
	}
}

type Result struct {
	Status Status
	Detail string
}

type Check struct {
	Name string
	Run  func(ctx context.Context, st *store.Store) Result
	Fix  func(ctx context.Context, st *store.Store) error
}

type CheckReport struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	FixApplied bool   `json:"fix_applied,omitempty"`
	FixError   string `json:"fix_error,omitempty"`
}

type Report struct {
	Checks []CheckReport `json:"checks"`
}

func (r Report) HasFailures() bool {
	for _, c := range r.Checks {
		if c.Status == Fail.String() {
			return true
		}
	}
	return false
}

// Run every check, read-only.
func Run(ctx context.Context, st *store.Store) Report {
	return iterate(ctx, st, false)
}

// Run every check, then apply the registered fix for each failed check whose Check has a non-nil Fix.
func Heal(ctx context.Context, st *store.Store) Report {
	return iterate(ctx, st, true)
}

func iterate(ctx context.Context, st *store.Store, attemptFix bool) Report {
	checks := AllChecks()
	out := Report{Checks: make([]CheckReport, 0, len(checks))}

	for _, c := range checks {
		res := c.Run(ctx, st)
		entry := CheckReport{Name: c.Name, Status: res.Status.String(), Detail: res.Detail}

		if attemptFix && res.Status == Fail && c.Fix != nil {
			applyFix(ctx, st, c, &entry)
		}

		out.Checks = append(out.Checks, entry)
	}
	return out
}

func applyFix(ctx context.Context, st *store.Store, c Check, entry *CheckReport) {
	if err := c.Fix(ctx, st); err != nil {
		entry.FixError = err.Error()
		return
	}

	entry.FixApplied = true
	rerun := c.Run(ctx, st)

	entry.Status = rerun.Status.String()
	entry.Detail = rerun.Detail
	if rerun.Status == Fail {
		entry.FixError = fmt.Sprintf("fix ran but check still fails: %s", rerun.Detail)
	}
}
