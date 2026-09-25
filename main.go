package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

var errUsage = errors.New("usage")

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprint(os.Stderr, usage())
			os.Exit(2)
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}

		fmt.Fprintf(os.Stderr, "harikube: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	kubectlArgs, err := buildKubectlArgs(args)
	if err != nil {
		return err
	}

	cmd := exec.Command("kubectl", kubectlArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func buildKubectlArgs(args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, errUsage
	}

	verb := args[0]
	rest := args[1:]

	switch verb {
	case "get", "watch", "create", "delete":
		return append([]string{verb}, rest...), nil
	case "list":
		return append([]string{"get"}, rest...), nil
	case "update":
		return append([]string{"apply"}, rest...), nil
	default:
		return nil, fmt.Errorf("%w: unsupported verb %q", errUsage, verb)
	}
}

func usage() string {
	return `Usage:
  kubectl harikube <verb> [arguments...]

Supported verbs:
  get     -> kubectl get
  list    -> kubectl get
  watch   -> kubectl watch
  create  -> kubectl create
  update  -> kubectl apply
  delete  -> kubectl delete
`
}
