package parser

import "testing"

var testProjects = []string{"test"}
var testPackages = []string{"test/aa", "test/ab", "test/ac/aa", "test/ac/ab", "test/ac/ac", "test/bb", "test/bc"}
var testTargets = []string{"test/aa:a", "test/ab:a", "test/ac/aa:aa", "test/ac/aa:ab", "test/ac/ab:a", "test/ac/ac:a", "test/bb:a", "test/bc:a"}

type SplitResult struct {
	Project    string
	Pkg        string
	Spread     string
	TargetName string
	Err        error
}

var testRunTargets = map[string]SplitResult{
	"//t": {
		Project: "t",
	},
	"//test": {
		Project: "test",
	},
	"//test/": {
		Project: "test",
	},
	"//test/a": {
		Project: "test",
		Pkg:     "a",
	},
	"//test/ac": {
		Project: "test",
		Pkg:     "ac",
	},
	"//test/ac/": {
		Project: "test",
		Pkg:     "ac/",
	},
	"//test/aa:": {
		Project: "test",
		Pkg:     "aa",
	},
	"//test/aa:a": {
		Project:    "test",
		Pkg:        "aa",
		TargetName: "a",
	},
	"//test/aa:ab": {
		Project:    "test",
		Pkg:        "aa",
		TargetName: "ab",
	},
}

func TestSplitTargetForAutocomplete(t *testing.T) {
	for target, res := range testRunTargets {
		proj, pkg, targetName, err := SplitTargetForAutocomplete(target)
		if proj != res.Project {
			t.Errorf("Project was incorrect for test %s, got: %s, want: %s.", target, proj, res.Project)
		}
		if pkg != res.Pkg {
			t.Errorf("Package was incorrect for test %s, got: %s, want: %s.", target, pkg, res.Pkg)
		}
		if targetName != res.TargetName {
			t.Errorf("TargetName was incorrect for test %s, got: %s, want: %s.", target, targetName, res.TargetName)
		}
		if err != res.Err {
			t.Errorf("Error was incorrect for test %s, got: %s, want: %s.", target, err, res.Err)
		}
	}
}
