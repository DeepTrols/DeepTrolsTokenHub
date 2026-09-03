// Package promrules holds the TH-P05-05 production alert pack and its
// structural verification. Fixture evaluation (firing / recovery semantics)
// is proven by promtool (tests/p05_alerts_test.yml); these Go tests enforce
// what promtool does not: the label-safety policy, rule completeness, the
// metric inventory boundary, and the runbook contract.
package promrules

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type ruleFile struct {
	Groups []struct {
		Name  string `yaml:"name"`
		Rules []struct {
			Alert       string            `yaml:"alert"`
			Expr        string            `yaml:"expr"`
			For         string            `yaml:"for"`
			Labels      map[string]string `yaml:"labels"`
			Annotations map[string]string `yaml:"annotations"`
		} `yaml:"rules"`
	} `yaml:"groups"`
}

const rulesPath = "tokenhub_p05.rules.yml"

// runbookPath is resolved from the package directory (ops/prometheus).
const runbookPath = "../../docs/RUNBOOK_ALERTS.md"

func loadRules(t *testing.T) ruleFile {
	t.Helper()
	raw, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("read rules: %v", err)
	}
	var rf ruleFile
	if err := yaml.Unmarshal(raw, &rf); err != nil {
		t.Fatalf("rules YAML is not syntactically valid: %v", err)
	}
	if len(rf.Groups) == 0 {
		t.Fatal("rules file defines no groups")
	}
	return rf
}

// knownMetrics is the complete vetted metric inventory (TH-P05-04 baseline
// + TH-P05-11 worker families + TH-P05-05 alert-support families). Alert
// expressions must never reference anything else.
var knownMetrics = map[string]bool{
	"gateway_requests_total": true, "gateway_success_total": true,
	"gateway_error_total": true, "gateway_request_duration_seconds": true,
	"billing_reserve_total": true, "billing_reserve_failed_total": true,
	"billing_settle_total": true, "billing_settle_failed_total": true,
	"billing_release_total": true, "billing_release_failed_total": true,
	"billing_undercharge_fallback_total": true, "billing_pricing_incomplete_total": true,
	"gateway_provider_blocked_before_call_total": true,
	"worker_cycles_total":                        true, "worker_cycle_duration_seconds": true,
	"worker_lease_total":                  true,
	"reconciliation_critical_diffs_total": true, "app_dependency_up": true,
}

// forbiddenTokens may never appear in expressions, label values or
// annotations (TH-P05-04/11 label policy, TH-P05-05 §14).
var forbiddenTokens = []string{
	"user_id", "request_id", "tenant_id", "order_no",
	"email", "api_key", "apikey", "bearer ", "jwt", "password",
	"hostname", "pod_id", "pod id", "secret",
}

// allowedMatcherLabels is the only label set an alert expression may match
// on (all vetted low-cardinality allowlists).
var allowedMatcherLabels = map[string]bool{
	"worker": true, "outcome": true, "dependency": true,
	"endpoint": true, "reason_class": true,
}

var metricRefPattern = regexp.MustCompile(`([a-z_][a-z0-9_]*(?:_[a-z0-9]+)*)\s*[\{\[]`)
var matcherBlockPattern = regexp.MustCompile(`\{([^}]*)\}`)
var matcherNamePattern = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*=~?`)
var ipv4Pattern = regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`)

// TestRules_StructurallyComplete asserts every rule carries the mandatory
// fields (name / expr / for / severity / summary / description / runbook).
func TestRules_StructurallyComplete(t *testing.T) {
	rf := loadRules(t)
	total := 0
	for _, g := range rf.Groups {
		for _, r := range g.Rules {
			total++
			if r.Alert == "" || !strings.HasPrefix(r.Alert, "TokenHub") {
				t.Errorf("rule %d: missing or non-TokenHub alert name %q", total, r.Alert)
			}
			if strings.TrimSpace(r.Expr) == "" {
				t.Errorf("%s: empty expr", r.Alert)
			}
			if r.For == "" {
				t.Errorf("%s: missing for-duration", r.Alert)
			}
			sev := r.Labels["severity"]
			if sev != "critical" && sev != "warning" {
				t.Errorf("%s: severity = %q, want critical|warning", r.Alert, sev)
			}
			for k := range r.Labels {
				if k != "severity" {
					t.Errorf("%s: unexpected rule label %q (policy: severity only)", r.Alert, k)
				}
			}
			for _, key := range []string{"summary", "description", "runbook"} {
				if r.Annotations[key] == "" {
					t.Errorf("%s: missing annotation %q", r.Alert, key)
				}
			}
		}
	}
	if total < 5 {
		t.Fatalf("expected at least the 5 mandated alert families, got %d rules", total)
	}
}

// TestRules_RequiredInventory pins that the mandated semantic families all
// exist (names may be tiered, so we check prefixes).
func TestRules_RequiredInventory(t *testing.T) {
	rf := loadRules(t)
	var names []string
	for _, g := range rf.Groups {
		for _, r := range g.Rules {
			names = append(names, r.Alert)
		}
	}
	joined := strings.Join(names, "\n")
	for _, required := range []string{
		"TokenHubBillingUndercharge",
		"TokenHubCriticalReconciliationDiff",
		"TokenHubWorkerSilent", // tiered: Fast / Reconciler / Hourly
		"TokenHubDatabaseUnavailable",
		"TokenHubRedisUnavailable",
	} {
		if !strings.Contains(joined, required) {
			t.Errorf("required alert family %s missing from inventory:\n%s", required, joined)
		}
	}
	// Worker silence must be tiered (different schedules -> different windows).
	if strings.Count(joined, "TokenHubWorkerSilent") < 2 {
		t.Error("worker silence alerting must be tiered per schedule, found a single rule")
	}
}

// TestRules_NoSensitiveOrUnknownContent is the TH-P05-05 §14 regression:
// expressions, labels and annotations never reintroduce identity-shaped or
// secret-shaped content, reference only the vetted metric inventory, and
// match only allowlisted labels.
func TestRules_NoSensitiveOrUnknownContent(t *testing.T) {
	rf := loadRules(t)
	for _, g := range rf.Groups {
		for _, r := range g.Rules {
			blobs := []string{r.Expr}
			for _, v := range r.Labels {
				blobs = append(blobs, v)
			}
			for _, v := range r.Annotations {
				blobs = append(blobs, v)
			}
			for _, blob := range blobs {
				lower := strings.ToLower(blob)
				for _, bad := range forbiddenTokens {
					if strings.Contains(lower, bad) {
						t.Errorf("%s: forbidden token %q present", r.Alert, bad)
					}
				}
				if ipv4Pattern.MatchString(blob) {
					t.Errorf("%s: IP address literal present", r.Alert)
				}
			}

			// Every metric referenced must belong to the vetted inventory.
			for _, m := range metricRefPattern.FindAllStringSubmatch(r.Expr, -1) {
				if !knownMetrics[m[1]] {
					t.Errorf("%s: expr references unknown metric %q", r.Alert, m[1])
				}
			}

			// Every matcher label must be an allowlisted low-cardinality label.
			for _, blk := range matcherBlockPattern.FindAllStringSubmatch(r.Expr, -1) {
				for _, mm := range matcherNamePattern.FindAllStringSubmatch(blk[1], -1) {
					if !allowedMatcherLabels[mm[1]] {
						t.Errorf("%s: expr matches on non-allowlisted label %q", r.Alert, mm[1])
					}
				}
			}
		}
	}
}

// workerWhitelist is the fixed leased-worker fleet vocabulary (TH-P05-11).
// Silence rules may only ever match this set — never a catch-all.
var workerWhitelist = map[string]bool{
	"health_checker": true, "billing_sync": true, "reconciler": true,
	"subscription_expirer": true, "subscription_renewer": true,
}

var workerMatcherPattern = regexp.MustCompile(`worker\s*=~?\s*"([^"]*)"`)

// TestRules_WorkerSilenceNeverRegexAll proves the worker silence rules use
// the fixed worker whitelist, never a catch-all like worker=~".*", and that
// every leased worker in the fleet is covered by some silence rule.
func TestRules_WorkerSilenceNeverRegexAll(t *testing.T) {
	rf := loadRules(t)
	covered := map[string]bool{}
	for _, g := range rf.Groups {
		for _, r := range g.Rules {
			if !strings.Contains(r.Alert, "WorkerSilent") {
				continue
			}
			if strings.Contains(r.Expr, `.*`) {
				t.Errorf("%s: worker silence rule uses a wildcard regex", r.Alert)
			}
			// Every referenced worker must be on the fixed whitelist.
			for _, mm := range workerMatcherPattern.FindAllStringSubmatch(r.Expr, -1) {
				for _, name := range strings.Split(mm[1], "|") {
					if !workerWhitelist[name] {
						t.Errorf("%s: worker matcher references non-whitelisted worker %q", r.Alert, name)
					}
					covered[name] = true
				}
			}
		}
	}
	// Fleet completeness: no leased worker may lack silence coverage.
	for w := range workerWhitelist {
		if !covered[w] {
			t.Errorf("leased worker %q is covered by no silence rule", w)
		}
	}
}

// TestRules_RunbookContract asserts the runbook document exists and carries
// a section for every alert the pack can raise.
func TestRules_RunbookContract(t *testing.T) {
	rf := loadRules(t)
	raw, err := os.ReadFile(runbookPath)
	if err != nil {
		t.Fatalf("runbook missing (required by every alert annotation): %v", err)
	}
	runbook := string(raw)
	for _, section := range []string{
		"Meaning", "Impact", "First checks", "Evidence", "Safe mitigation", "Escalation", "Recovery verification",
	} {
		if !strings.Contains(runbook, section) {
			t.Errorf("runbook missing required section %q", section)
		}
	}
	// Money-safety red line: the runbook must never recommend direct wallet
	// mutation; it must keep the Diff -> Human Review -> Explicit Adjustment
	// -> Wallet Service -> Ledger path.
	if !strings.Contains(runbook, "Human Review") {
		t.Error("runbook must document the human-review adjustment path")
	}
	for _, g := range rf.Groups {
		for _, r := range g.Rules {
			heading := "### " + r.Alert
			if !strings.Contains(runbook, heading) {
				t.Errorf("runbook has no section for %s", r.Alert)
			}
		}
	}
}
