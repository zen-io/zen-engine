package parser

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/zen-io/zen-core/target"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/exp/slices"
)

func (pp *PackageParser) AutocompleteTarget(target string) ([]string, error) {
	autocompleteOpts := []string{}

	if len(target) < 2 {
		if target != "" && !regexp.MustCompile(`\/+`).MatchString(target) {
			return []string{}, nil
		} else {
			return []string{"//"}, nil
		}
	}

	project, pkg, targetName, err := SplitTargetForAutocomplete(target)
	if err != nil {
		return nil, err
	}

	// complete just the project
	if len(pkg) == 0 && len(targetName) == 0 && !strings.HasSuffix(target, "/") {
		opts := pp.autocompleteProject(project)
		if len(opts) == 1 && opts[0] == target {
			autocompleteOpts = []string{opts[0] + "/"}
		} else {
			autocompleteOpts = opts
		}
		return autocompleteOpts, nil
	}

	// if the target does not contain a : character, attempt to autocomplete packages
	if !strings.Contains(target, ":") {
		if complete, err := pp.autocompletePackage(project, pkg); err != nil {
			return nil, err
		} else { // we only want the next part of the path, not all of it
			autocompleteOpts = append(autocompleteOpts, complete...)
		}
	}

	// if more than one options is identified, add a spread operator
	if len(autocompleteOpts) > 1 && strings.HasSuffix(target, "/") {
		autocompleteOpts = append([]string{fmt.Sprintf("%s/...", autocompleteOpts[0][:strings.LastIndex(autocompleteOpts[0], "/")])}, autocompleteOpts...)
	} else if len(autocompleteOpts) == 1 && autocompleteOpts[0] == target {
		autocompleteOpts[0] = autocompleteOpts[0] + "/"
	}

	// if the autocomplete does not end with /, attempt to load target names for the package
	if !strings.HasSuffix(target, "/") {
		if complete, err := pp.autocompleteTargetNamesForPackage(project, pkg, targetName); err != nil {
			return nil, err
		} else if len(complete) > 0 {
			autocompleteOpts = append(autocompleteOpts, complete...)
		}
	}

	sort.Strings(autocompleteOpts)

	return autocompleteOpts, nil
}

// Get project names starting with "project", when autocompleting a target
func (pp *PackageParser) autocompleteProject(project string) []string {
	autocompleteOpts := []string{}
	for proj := range pp.projects {
		if strings.HasPrefix(proj, project) {
			autocompleteOpts = append(autocompleteOpts, fmt.Sprintf("//%s", proj))
		}
	}

	return autocompleteOpts
}

func (pp *PackageParser) searchPackage(project, pkg string) ([]string, []string, error) {
	autocompleteOpts := []string{}
	allPkgs := []string{}

	projPath := pp.projects[project].Path
	projFilename := pp.projects[project].Config.Parse.Filename

	for _, placementOpt := range pp.projects[project].Config.Parse.Placement {
		searchPath := filepath.Join(projPath, strings.ReplaceAll(placementOpt, "{PKG}", pkg+"*"), "**", projFilename)

		optPrefix := filepath.Dir(strings.ReplaceAll(placementOpt, "{PKG}", pkg)) + "/"
		if optPrefix == "./" {
			optPrefix = ""
		}

		fsys, pat := doublestar.SplitPattern(searchPath)
		if err := doublestar.GlobWalk(os.DirFS(fsys), pat, func(p string, d fs.DirEntry) error {
			split := strings.Split(p, "/")

			opt := fmt.Sprintf("//%s/%s%s", project, optPrefix, split[0])
			allPkgs = append(allPkgs, fmt.Sprintf("//%s/%s%s", project, optPrefix, filepath.Join(split[:len(split)-1]...)))

			if !slices.Contains(autocompleteOpts, opt) && len(split) > 0 {
				autocompleteOpts = append(autocompleteOpts, opt)
			}

			return nil
		}, doublestar.WithFailOnIOErrors()); err != nil {
			return nil, nil, fmt.Errorf("glob walking %s: %w", fsys, err)
		}
	}

	return allPkgs, autocompleteOpts, nil
}

func (pp *PackageParser) autocompletePackage(project, pkg string) ([]string, error) {
	_, autocompleteOpts, err := pp.searchPackage(project, pkg)
	if err != nil {
		return nil, err
	}

	return autocompleteOpts, nil
}

func (pp *PackageParser) autocompleteTargetNamesForPackage(project, pkg, targetName string) ([]string, error) {
	autocompleteOpts := []string{}

	if err := pp.ParsePackageTargets(project, pkg); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("autocompleting //%s/%s: %w", project, pkg, err)
		}
	} else {
		pkgTargets, err := pp.GetAllTargetsInPackage(project, pkg)
		if err != nil {
			return nil, err
		} else {
			for _, t := range pkgTargets {
				if strings.HasPrefix(t.Name, targetName) {
					autocompleteOpts = append(autocompleteOpts, fmt.Sprintf("//%s/%s:%s", project, pkg, t.Name))
				}
			}
		}
	}

	return autocompleteOpts, nil
}

// Extracts a project, pkg, spread operator and target name from a target using a regex
// target may contain any of the combinations:
// - just project
// - project and pkg
// - project, pkg and target name
func SplitTargetForAutocomplete(target string) (project string, pkg string, targetName string, err error) {
	re := regexp.MustCompile(`^\/\/(?:([\w\d_\-]+))?\/?(?:((?:[\/\w\d_\-]+)*))?(?:\:([\w\d_\-]+)?)?$`)

	matches := re.FindStringSubmatch(target)
	if len(matches) == 0 {
		err = fmt.Errorf("target %s is not valid", target)
		return
	}

	project = matches[1]
	pkg = matches[2]
	targetName = matches[3]

	return
}

func (pp *PackageParser) ExpandTargets(targets []string, defaultScript string) ([]string, error) {
	finalTargets := []string{}
	for len(targets) > 0 {
		item := targets[0]
		targets = targets[1:]

		spreadRe := regexp.MustCompile(`^\/\/([\w\d\_\.\-]+)\/(?:([\w\d\_\.\-\/]+)\/)*\.\.\.(:.+)?$`)
		allRe := regexp.MustCompile(`^\/\/([\w\d\_\.\-]+)\/?(?:([\w\d\_\.\-\/]+)*)*:all(:.+)?$`)
		onlyPkgRe := regexp.MustCompile(`^\/\/([\w\d\_\.\-]+)\/?([\w\d\_\.\-\/]+)*$`)

		if spreadMatches := spreadRe.FindStringSubmatch(item); len(spreadMatches) > 0 { // spread operator
			project := spreadMatches[1]
			pkg := spreadMatches[2]
			var script string
			if len(spreadMatches) > 2 {
				script = spreadMatches[3]
			} else {
				script = defaultScript
			}

			if pp.projects[project] == nil {
				keys := []string{}
				for k := range pp.projects {
					keys = append(keys, k)
				}
				return nil, fmt.Errorf("project %s not configured. known projects are: %s", project, strings.Join(keys, ", "))
			}

			result, _, err := pp.searchPackage(project, pkg)
			if err != nil {
				return nil, err
			}

			for _, r := range result {
				targets = append(targets, fmt.Sprintf("%s:all%s", r, script))
			}
		} else if allMatches := allRe.FindStringSubmatch(item); len(allMatches) > 0 { // :all package
			project := allMatches[1]
			pkg := allMatches[2]

			var script string
			if len(allMatches) > 2 && len(allMatches[3]) > 0 {
				script = allMatches[3]
			} else {
				script = defaultScript
			}

			result, err := pp.GetAllTargetsInPackage(project, pkg)
			if err != nil {
				return nil, err
			}

			for _, r := range result {
				finalTargets = append(finalTargets, fmt.Sprintf("%s:%s", r.Qn(), script))
			}
		} else if onlyPkgRe.MatchString(item) { // pkg without target or spread
			targets = append(targets, fmt.Sprintf("%s:all", item))
		} else {
			ensureFqn, err := target.NewFqnFromStr(item)
			ensureFqn.SetDefaultScript(defaultScript)

			if err != nil {
				return nil, err
			}
			finalTargets = append(finalTargets, ensureFqn.Fqn())
		}
	}
	return finalTargets, nil
}
