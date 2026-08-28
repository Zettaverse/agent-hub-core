package flow

import "testing"

func eval(t *testing.T, expr string, vars map[string]any) any {
	t.Helper()
	v, err := evalExpression(expr, vars)
	if err != nil {
		t.Fatalf("eval %q: %v", expr, err)
	}
	return v
}

func TestExprLiteralsAndComparison(t *testing.T) {
	cases := []struct {
		expr string
		want bool
	}{
		{"1 == 1", true},
		{"1 != 2", true},
		{"2 > 1", true},
		{"1 < 2", true},
		{"2 >= 2", true},
		{"1 <= 2", true},
		{"3 > 5", false},
		{"true == true", true},
		{`"a" == "a"`, true},
		{`"a" != "b"`, true},
	}
	for _, tc := range cases {
		if got := eval(t, tc.expr, nil); got != tc.want {
			t.Errorf("eval %q = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestExprLogical(t *testing.T) {
	cases := []struct {
		expr string
		want any
	}{
		{"true && false", false},
		{"true || false", true},
		{"!true", false},
		{"!false", true},
		{"1 > 0 && 2 > 1", true},
		{"1 > 2 || 2 > 1", true},
		{"(1 == 1) && (2 == 2)", true},
	}
	for _, tc := range cases {
		if got := eval(t, tc.expr, nil); got != tc.want {
			t.Errorf("eval %q = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestExprVariablesAndFields(t *testing.T) {
	vars := map[string]any{
		"result": map[string]any{
			"value":   float64(20),
			"name":    "hello",
			"enabled": true,
			"items":   []any{1, 2, 3},
		},
	}
	cases := []struct {
		expr string
		want any
	}{
		{"result.value > 10", true},
		{"result.value == 20", true},
		{"result.name == \"hello\"", true},
		{"result.enabled == true", true},
		{"result.value", float64(20)},
		{"result.name.length", 5},
		{"result.items.length", 3},
		{"result.enabled", true},
	}
	for _, tc := range cases {
		if got := eval(t, tc.expr, vars); got != tc.want {
			t.Errorf("eval %q = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestExprShortCircuit(t *testing.T) {
	// The right side references an unknown variable; short-circuit must
	// prevent evaluation from failing.
	v := eval(t, "true || unknown_var == 1", nil)
	if v != true {
		t.Fatalf("short-circuit or = %v, want true", v)
	}
	v = eval(t, "false && unknown_var == 1", nil)
	if v != false {
		t.Fatalf("short-circuit and = %v, want false", v)
	}
}

func TestExprErrors(t *testing.T) {
	cases := []string{
		"unknown_var == 1",
		"1 +",
		"",
		"1 >",
		"1 &&",
		"(1 == 1",
	}
	for _, expr := range cases {
		if _, err := evalExpression(expr, nil); err == nil {
			t.Errorf("expected error for %q", expr)
		}
	}
}
