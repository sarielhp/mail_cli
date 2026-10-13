package calendar

import (
	"strings"
	"time"

	"github.com/teambition/rrule-go"
)

// NextOccurrences returns up to count upcoming occurrences of the event on or after from.
// For non-recurring events, it returns StartParsed if it falls on or after from.
// If count <= 0 or the event has no valid start time, it returns nil.
func (ev *ICSEvent) NextOccurrences(from time.Time, count int) []time.Time {
	if ev == nil || count <= 0 || ev.StartParsed.IsZero() {
		return nil
	}
	if ev.RRule == "" {
		if !ev.StartParsed.Before(from) {
			return []time.Time{ev.StartParsed}
		}
		return nil
	}

	rule := ev.buildRRule()
	if rule == nil {
		if !ev.StartParsed.Before(from) {
			return []time.Time{ev.StartParsed}
		}
		return nil
	}

	return queryRuleOccurrences(rule, from, count)
}

func (ev *ICSEvent) buildRRule() *rrule.RRule {
	rruleStr := strings.TrimSpace(ev.RRule)
	rruleStr = strings.TrimPrefix(rruleStr, "RRULE:")
	loc := ev.StartParsed.Location()
	if loc == nil {
		loc = time.UTC
	}

	opt, err := rrule.StrToROptionInLocation(rruleStr, loc)
	if err == nil {
		opt.Dtstart = ev.StartParsed
		rule, newErr := rrule.NewRRule(*opt)
		if newErr == nil {
			return rule
		}
	}

	rule, err := rrule.StrToRRule(rruleStr)
	if err != nil {
		return nil
	}
	rule.DTStart(ev.StartParsed)
	return rule
}

func queryRuleOccurrences(rule *rrule.RRule, from time.Time, count int) []time.Time {
	var occurrences []time.Time
	curr := from
	inc := true
	for len(occurrences) < count {
		next := rule.After(curr, inc)
		if next.IsZero() {
			break
		}
		occurrences = append(occurrences, next)
		curr = next
		inc = false
	}
	return occurrences
}
