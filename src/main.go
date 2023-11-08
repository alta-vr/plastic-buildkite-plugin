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
	return "nothing", nil

	//out, err := exec.Command("buildkite-agent", "meta-data", "set", name, value).CombinedOutput()
	//return string(out), err
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

func getComment(changeset int) (string, error) {
	out, err := exec.Command("cm", "log", fmt.Sprintf("cs:%d", changeset), "--csformat={comment}").CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func getFriendlyBranchName(branchName string) (string, error) {
	if strings.Contains(branchName, "-") {
		return "", errors.New("dashes not allowed")
	}

	if strings.HasSuffix(branchName, "/") {
		return "", errors.New("branch must not end with /")
	}

	branchName = strings.TrimPrefix(branchName, "/")
	branchName = strings.Replace(branchName, "/", "-", -1)
	return branchName, nil
}

func getChangeset(branchName string) (int, error) {
	revision := os.Getenv("BUILDKITE_COMMIT")
	if revision == "" || revision == "HEAD" {
		var err error
		revision, err = getHead(branchName)
		if err != nil {
			return -1, err
		}
	}

	if cs, err := strconv.Atoi(revision); err != nil || cs < 1 {
		return -1, err
	} else {
		return cs, nil
	}
}

func getUpdateTarget() (string, error) {
	alreadyInitialised, _ := getMetadata("lightforge:plastic:initialised", "false")
	if alreadyInitialised == "true" {
		plasticBranch, _ := getMetadata("lightforge:plastic:branch", "")
		plasticCs, _ := getMetadata("lightforge:plastic:changeset", "")
		fmt.Printf("using br:%s and cs:%s from metadata\n", plasticBranch, plasticCs)

		return fmt.Sprintf("cs:%s", plasticCs), nil
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

	changeset, err := getChangeset(branchName)
	if err != nil {
		return "", fmt.Errorf("Invalid changeset `%d` specified: %v\n", changeset, err)
	}

	// Set metadata before updating, as updating can take minutes.
	comment, err := getComment(changeset)
	if err != nil {
		return "", fmt.Errorf("Failed to get comment for `%v:%s`\n%v\n%s\n", changeset, branchName, err, comment)
	}

	if out, err := setMetadata("lightforge:plastic:branch", branchName); err != nil {
		return "", fmt.Errorf("Failed to set branch metadata: : %v.\n%s\n", err, out)
	}

	if out, err := setMetadata("lightforge:plastic:displaybranch", friendlyBranchName); err != nil {
		return "", fmt.Errorf("Failed to set branch metadata: : %v.\n%s\n", err, string(out))
	}

	if out, err := setMetadata("lightforge:plastic:changeset", strconv.Itoa(changeset)); err != nil {
		return "", fmt.Errorf("Failed to set changeset metadata: : %v.\n%s\n", err, string(out))
	}

	commitMetadata := fmt.Sprintf("commit %d\n\n\t%s", changeset, comment)
	if out, err := setMetadata("buildkite:git:commit", commitMetadata); err != nil {
		return "", fmt.Errorf("Failed to set buildkite:git:commit metadata: : %v.\n%s\n", err, string(out))
	}

	return fmt.Sprintf("cs:%d", changeset), nil
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
