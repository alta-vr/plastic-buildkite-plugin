package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func setMetadata(name string, value string) (string, error) {
	out, err := exec.Command("buildkite-agent", "meta-data", "set", name, value).CombinedOutput()
	return string(out), err
}

func getMetadata(name string, defaultValue string) (string, error) {
	out, err := exec.Command("buildkite-agent", "meta-data", "get", name, "--default", defaultValue).CombinedOutput()
	if err == nil {
		return string(out), err
	} else {
		return "", err
	}
}

func annotate(style, context, message string) error {
	_, err := exec.Command("buildkite-agent", "annotate", message, "--context", context, "--style", style).CombinedOutput()
	return err
}

func getHead(branch string) (string, error) {
	out, err := exec.Command("cm", "find", "changeset", fmt.Sprintf(`where branch = '%s'`, branch), `--format={changesetid}`, "order", "by", "changesetId", "desc", "LIMIT", "1", "--nototal").CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func getComment(selector string) (string, error) {

	if strings.Contains(selector, "sh:") {
		//Shelves must be handled differently to changesets
		out, err := exec.Command("cm", "find", "shelve", fmt.Sprintf(`where shelveid = '%s'`, selector), `--format={comment}`, "LIMIT", "1", "--nototal").CombinedOutput()
		return strings.TrimSpace(string(out)), err
	} 

	out, err := exec.Command("cm", "log", selector, "--csformat={comment}").CombinedOutput()
	return strings.TrimSpace(string(out)), err	
}

func getFriendlyBranchName(branchName string) (string, error) {

	if strings.HasSuffix(branchName, "/") {
		return "", errors.New("branch must not end with /")
	}

	branchName = strings.TrimPrefix(branchName, "/")
	branchName = strings.Replace(branchName, "/", "__", -1)
	return branchName, nil
}

func getSelector(branchName string) (string, error) {
	revision := os.Getenv("BUILDKITE_COMMIT")
	if revision == "" || revision == "HEAD" {
		var err error
		revision, err = getHead(branchName)
		if err != nil {
			return fmt.Sprintf("cs:%d", revision), err
		}

		return revision, err;
	}

	if strings.Contains(revision, ":") {
		//This adds support for specifying a shelf in the commit field (eg. sh:123)
		return revision, nil
	}

	if cs, err := strconv.Atoi(revision); err != nil || cs < 1 {
		return revision, err
	} else {
		return fmt.Sprintf("cs:%d", cs), nil
	}
}

func getUpdateTarget() (string, error) {
	alreadyInitialised, _ := getMetadata("lightforge:plastic:initialised", "false")
	if alreadyInitialised == "true" {
		plasticBranch, _ := getMetadata("lightforge:plastic:branch", "")
		selector, _ := getMetadata("lightforge:plastic:selector", "")
		fmt.Printf("using br:%s and %s from metadata\n", plasticBranch, selector)

		return selector, nil
	}

	if _, err := setMetadata("lightforge:plastic:initialised", "true"); err != nil {
		return "", errors.New("failed to set initialized metadata")
	}

	// Figure out our metadata
	// Start by getting the branch
	branchName := os.Getenv("BUILDKITE_BRANCH")

	friendlyBranchName, err := getFriendlyBranchName(branchName)
	if err != nil {
		return "", err
	}

	selector, err := getSelector(branchName)
	if err != nil {
		return "", fmt.Errorf("Invalid selector `%d` specified: %v\n", selector, err)
	}

	// Set metadata before updating, as updating can take minutes.
	comment, err := getComment(selector)
	if err != nil {
		return "", fmt.Errorf("Failed to get comment for `%v:%s`\n%v\n%s\n", selector, branchName, err, comment)
	}

	if out, err := setMetadata("lightforge:plastic:branch", branchName); err != nil {
		return "", fmt.Errorf("Failed to set branch metadata: : %v.\n%s\n", err, out)
	}

	if out, err := setMetadata("lightforge:plastic:displaybranch", friendlyBranchName); err != nil {
		return "", fmt.Errorf("Failed to set branch metadata: : %v.\n%s\n", err, string(out))
	}

	if out, err := setMetadata("lightforge:plastic:selector", selector); err != nil {
		return "", fmt.Errorf("Failed to set selector metadata: : %v.\n%s\n", err, string(out))
	}

	commitMetadata := fmt.Sprintf("commit %d\n\n\t%s", selector, comment)
	if out, err := setMetadata("buildkite:git:commit", commitMetadata); err != nil {
		return "", fmt.Errorf("Failed to set buildkite:git:commit metadata: : %v.\n%s\n", err, string(out))
	}

	return selector, nil
}

func exitAndError(message string) {
	fmt.Println(message)
	annotate("error", "lightforge-plastic-plugin", message)
	os.Exit(1)
}

func main() {
	cd, _ := os.Getwd()

	fmt.Println("Executing plastic-buildkite-plugin from " + cd)

	repoPath := os.Getenv("BUILDKITE_PLUGIN_PLASTIC_REPO")
	pipelineName := os.Getenv("BUILDKITE_PIPELINE_NAME")

	workspaceName, found := os.LookupEnv("BUILDKITE_PLUGIN_PLASTIC_WORKSPACENAME")
	if !found {
		workspaceName = fmt.Sprintf("buildkite-%s", pipelineName)
	}

	fmt.Printf("Creating workspace %q for repository %q\n", workspaceName, repoPath)
	if out, err := exec.Command("cm", "workspace", "create", workspaceName, ".", repoPath).CombinedOutput(); err != nil {
		if !strings.Contains(string(out), "already exists.") {
			exitAndError(fmt.Sprintf("Failed to create workspace `%s`: %v.\n%s\n", workspaceName, err, string(out)))
		}
	}

	target, err := getUpdateTarget()
	if err != nil {
		exitAndError(fmt.Sprintf("Failed to get target: %v\n", err))
	}

	fmt.Println("Cleaning workspace of any changes...")
	if out, err := exec.Command("cm", "undo", ".", "-R").CombinedOutput(); err != nil {
		exitAndError(fmt.Sprintf("Failed to undo changes: : %v.\n%s\n", err, string(out)))
	}

	fmt.Println("Setting workspace to " + target)
	if out, err := exec.Command("cm", "switch", target).CombinedOutput(); err != nil {
		exitAndError(fmt.Sprintf("Failed to update workspace: : %v.\n%s\n", err, string(out)))
	}

	fmt.Println("Update complete.")
}
