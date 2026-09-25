package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

var errUsage = errors.New("usage")

type kubectlRunner interface {
	run(stdin io.Reader, stdout, stderr io.Writer, args ...string) error
	output(args ...string) ([]byte, error)
}

type execKubectlRunner struct{}

func (execKubectlRunner) run(stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	cmd := exec.Command("kubectl", args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (execKubectlRunner) output(args ...string) ([]byte, error) {
	cmd := exec.Command("kubectl", args...)
	return cmd.Output()
}

type cliConfig struct {
	verb          string
	globalArgs    []string
	namespace     string
	allNamespaces bool
	selector      string
	fieldSelector string
	filename      string
	output        string
	resource      string
	name          string
	watch         bool
	manifestStdin bool
	manifestBytes []byte
}

type manifestMeta struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
}

type apiGroupList struct {
	Groups []struct {
		Name     string `json:"name"`
		Versions []struct {
			GroupVersion string `json:"groupVersion"`
			Version      string `json:"version"`
		} `json:"versions"`
		PreferredVersion struct {
			GroupVersion string `json:"groupVersion"`
			Version      string `json:"version"`
		} `json:"preferredVersion"`
	} `json:"groups"`
}

type apiResourceList struct {
	GroupVersion string        `json:"groupVersion"`
	APIResources []apiResource `json:"resources"`
}

type apiResource struct {
	Name         string   `json:"name"`
	SingularName string   `json:"singularName"`
	Namespaced   bool     `json:"namespaced"`
	Kind         string   `json:"kind"`
	ShortNames   []string `json:"shortNames"`
	Verbs        []string `json:"verbs"`
}

type resourceRef struct {
	Group      string
	Version    string
	Resource   string
	Kind       string
	Namespaced bool
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, execKubectlRunner{}); err != nil {
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

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, kubectl kubectlRunner) error {
	cfg, err := parseArgs(args)
	if err != nil {
		return err
	}

	if cfg.filename != "" {
		cfg.manifestBytes, cfg.manifestStdin, err = loadManifest(cfg.filename, stdin)
		if err != nil {
			return err
		}
	}

	switch cfg.verb {
	case "get", "list", "watch", "delete":
		return runResourceVerb(cfg, stdout, stderr, kubectl)
	case "create", "update":
		return runManifestVerb(cfg, stdout, stderr, kubectl)
	default:
		return fmt.Errorf("%w: unsupported verb %q", errUsage, cfg.verb)
	}
}

func parseArgs(args []string) (cliConfig, error) {
	if len(args) == 0 {
		return cliConfig{}, errUsage
	}

	cfg := cliConfig{verb: args[0]}
	positionals := make([]string, 0, 2)

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-n" || arg == "--namespace":
			value, next, err := consumeFlagValue(arg, args, i)
			if err != nil {
				return cliConfig{}, err
			}
			cfg.namespace = value
			i = next
		case strings.HasPrefix(arg, "--namespace="):
			cfg.namespace = strings.TrimPrefix(arg, "--namespace=")
		case arg == "-A" || arg == "--all-namespaces":
			cfg.allNamespaces = true
		case arg == "-l" || arg == "--selector":
			value, next, err := consumeFlagValue(arg, args, i)
			if err != nil {
				return cliConfig{}, err
			}
			cfg.selector = value
			i = next
		case strings.HasPrefix(arg, "--selector="):
			cfg.selector = strings.TrimPrefix(arg, "--selector=")
		case arg == "--field-selector":
			value, next, err := consumeFlagValue(arg, args, i)
			if err != nil {
				return cliConfig{}, err
			}
			cfg.fieldSelector = value
			i = next
		case strings.HasPrefix(arg, "--field-selector="):
			cfg.fieldSelector = strings.TrimPrefix(arg, "--field-selector=")
		case arg == "-f" || arg == "--filename":
			value, next, err := consumeFlagValue(arg, args, i)
			if err != nil {
				return cliConfig{}, err
			}
			cfg.filename = value
			i = next
		case strings.HasPrefix(arg, "--filename="):
			cfg.filename = strings.TrimPrefix(arg, "--filename=")
		case arg == "-o" || arg == "--output":
			value, next, err := consumeFlagValue(arg, args, i)
			if err != nil {
				return cliConfig{}, err
			}
			cfg.output = value
			i = next
		case strings.HasPrefix(arg, "--output="):
			cfg.output = strings.TrimPrefix(arg, "--output=")
		case isSupportedGlobalFlag(arg):
			value, next, err := maybeConsumeGlobalFlag(args, i)
			if err != nil {
				return cliConfig{}, err
			}
			cfg.globalArgs = append(cfg.globalArgs, value...)
			i = next
		case strings.HasPrefix(arg, "-"):
			return cliConfig{}, fmt.Errorf("%w: unsupported flag %q", errUsage, arg)
		default:
			positionals = append(positionals, arg)
		}
	}

	cfg.watch = cfg.verb == "watch"

	switch cfg.verb {
	case "get", "list", "watch", "delete":
		if len(positionals) == 0 && cfg.filename == "" {
			return cliConfig{}, fmt.Errorf("%w: missing resource", errUsage)
		}
		if cfg.filename != "" {
			if cfg.verb != "delete" || len(positionals) != 0 {
				return cliConfig{}, fmt.Errorf("%w: delete by file does not accept a resource argument", errUsage)
			}
			return cfg, nil
		}
		cfg.resource = positionals[0]
		if len(positionals) > 1 {
			cfg.name = positionals[1]
		}
		if len(positionals) > 2 {
			return cliConfig{}, fmt.Errorf("%w: too many positional arguments", errUsage)
		}
	case "create", "update":
		if cfg.filename == "" {
			return cliConfig{}, fmt.Errorf("%w: %s requires -f/--filename", errUsage, cfg.verb)
		}
		if len(positionals) != 0 {
			return cliConfig{}, fmt.Errorf("%w: %s only supports -f/--filename", errUsage, cfg.verb)
		}
	default:
		return cliConfig{}, fmt.Errorf("%w: unsupported verb %q", errUsage, cfg.verb)
	}

	return cfg, nil
}

func consumeFlagValue(flag string, args []string, index int) (string, int, error) {
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("%w: missing value for %s", errUsage, flag)
	}
	return args[index+1], index + 1, nil
}

func isSupportedGlobalFlag(arg string) bool {
	if globalFlagsWithValue[arg] || globalBoolFlags[arg] {
		return true
	}
	for flag := range globalFlagsWithValue {
		if strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	for flag := range globalBoolFlags {
		if strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}

func maybeConsumeGlobalFlag(args []string, index int) ([]string, int, error) {
	arg := args[index]
	if globalBoolFlags[arg] || hasInlineAssignment(arg) {
		return []string{arg}, index, nil
	}
	if !globalFlagsWithValue[arg] {
		return nil, index, fmt.Errorf("%w: unsupported flag %q", errUsage, arg)
	}
	if index+1 >= len(args) {
		return nil, index, fmt.Errorf("%w: missing value for %s", errUsage, arg)
	}
	return []string{arg, args[index+1]}, index + 1, nil
}

func hasInlineAssignment(arg string) bool {
	return strings.Contains(arg, "=")
}

var globalFlagsWithValue = map[string]bool{
	"--as":                    true,
	"--as-group":              true,
	"--as-uid":                true,
	"--as-user-extra":         true,
	"--cache-dir":             true,
	"--certificate-authority": true,
	"--client-certificate":    true,
	"--client-key":            true,
	"--cluster":               true,
	"--context":               true,
	"--kubeconfig":            true,
	"--password":              true,
	"--profile":               true,
	"--profile-output":        true,
	"--proxy-url":             true,
	"--request-timeout":       true,
	"--server":                true,
	"-s":                      true,
	"--tls-server-name":       true,
	"--token":                 true,
	"--user":                  true,
	"--username":              true,
	"--v":                     true,
	"--vmodule":               true,
}

var globalBoolFlags = map[string]bool{
	"--disable-compression":      true,
	"--insecure-skip-tls-verify": true,
	"--match-server-version":     true,
	"--warnings-as-errors":       true,
}

func loadManifest(filename string, stdin io.Reader) ([]byte, bool, error) {
	if filename == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, true, err
		}
		return data, true, nil
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, false, err
	}
	return data, false, nil
}

func runResourceVerb(cfg cliConfig, stdout, stderr io.Writer, kubectl kubectlRunner) error {
	if cfg.filename != "" {
		return runManifestDelete(cfg, stdout, stderr, kubectl)
	}

	resource, err := discoverResource(cfg.globalArgs, cfg.resource, "", kubectl)
	if err != nil {
		return err
	}

	namespace := cfg.namespace
	if resource.Namespaced && !cfg.allNamespaces {
		namespace, err = resolveNamespace(namespace, cfg.globalArgs, kubectl)
		if err != nil {
			return err
		}
	}

	path, err := buildResourcePath(resource, namespace, cfg.allNamespaces, cfg.name, cfg.selector, cfg.fieldSelector, cfg.watch)
	if err != nil {
		return err
	}

	return runRawRequest(cfg, path, nil, stdout, stderr, kubectl)
}

func runManifestVerb(cfg cliConfig, stdout, stderr io.Writer, kubectl kubectlRunner) error {
	manifest, err := parseManifestMeta(cfg.manifestBytes)
	if err != nil {
		return err
	}

	resource, err := discoverResource(cfg.globalArgs, manifest.Kind, manifest.APIVersion, kubectl)
	if err != nil {
		return err
	}

	namespace := manifest.Metadata.Namespace
	if cfg.namespace != "" {
		namespace = cfg.namespace
	}
	if resource.Namespaced {
		namespace, err = resolveNamespace(namespace, cfg.globalArgs, kubectl)
		if err != nil {
			return err
		}
	}

	name := ""
	if cfg.verb == "update" {
		name = manifest.Metadata.Name
		if name == "" {
			return fmt.Errorf("%w: update manifest must include metadata.name", errUsage)
		}
	}

	path, err := buildResourcePath(resource, namespace, false, name, "", "", false)
	if err != nil {
		return err
	}

	return runRawRequest(cfg, path, bytes.NewReader(cfg.manifestBytes), stdout, stderr, kubectl)
}

func runManifestDelete(cfg cliConfig, stdout, stderr io.Writer, kubectl kubectlRunner) error {
	manifest, err := parseManifestMeta(cfg.manifestBytes)
	if err != nil {
		return err
	}

	resource, err := discoverResource(cfg.globalArgs, manifest.Kind, manifest.APIVersion, kubectl)
	if err != nil {
		return err
	}

	if manifest.Metadata.Name == "" {
		return fmt.Errorf("%w: delete manifest must include metadata.name", errUsage)
	}

	namespace := manifest.Metadata.Namespace
	if cfg.namespace != "" {
		namespace = cfg.namespace
	}
	if resource.Namespaced {
		namespace, err = resolveNamespace(namespace, cfg.globalArgs, kubectl)
		if err != nil {
			return err
		}
	}

	path, err := buildResourcePath(resource, namespace, false, manifest.Metadata.Name, "", "", false)
	if err != nil {
		return err
	}

	return runRawRequest(cfg, path, nil, stdout, stderr, kubectl)
}

func runRawRequest(cfg cliConfig, path string, stdin io.Reader, stdout, stderr io.Writer, kubectl kubectlRunner) error {
	var verb string
	switch cfg.verb {
	case "get", "list", "watch":
		verb = "get"
	case "create":
		verb = "create"
	case "update":
		verb = "replace"
	case "delete":
		verb = "delete"
	default:
		return fmt.Errorf("%w: unsupported verb %q", errUsage, cfg.verb)
	}

	args := append([]string{}, cfg.globalArgs...)
	args = append(args, verb, "--raw", path)

	if stdin != nil {
		args = append(args, "-f", "-")
	}

	if cfg.output != "" {
		if cfg.watch {
			return fmt.Errorf("%w: watch does not support --output", errUsage)
		}
		data, err := kubectl.output(args...)
		if err != nil {
			return err
		}
		return renderOutput(data, cfg.output, stdout)
	}

	return kubectl.run(stdin, stdout, stderr, args...)
}

func renderOutput(data []byte, format string, stdout io.Writer) error {
	switch format {
	case "", "json":
		_, err := stdout.Write(data)
		return err
	case "yaml":
		var decoded any
		if err := json.Unmarshal(data, &decoded); err != nil {
			return fmt.Errorf("failed to decode JSON response: %w", err)
		}
		yml, err := yaml.Marshal(decoded)
		if err != nil {
			return fmt.Errorf("failed to encode YAML response: %w", err)
		}
		_, err = stdout.Write(yml)
		return err
	default:
		return fmt.Errorf("%w: unsupported output format %q", errUsage, format)
	}
}

func resolveNamespace(namespace string, globalArgs []string, kubectl kubectlRunner) (string, error) {
	if namespace != "" {
		return namespace, nil
	}

	args := append([]string{}, globalArgs...)
	args = append(args, "config", "view", "--minify", "-o", "jsonpath={..namespace}")
	data, err := kubectl.output(args...)
	if err != nil {
		return "", err
	}

	resolved := strings.TrimSpace(string(data))
	if resolved == "" {
		return "default", nil
	}
	return resolved, nil
}

func discoverResource(globalArgs []string, token, apiVersion string, kubectl kubectlRunner) (resourceRef, error) {
	var groups apiGroupList
	if err := kubectlJSON(kubectl, &groups, append(globalArgs, "get", "--raw", "/apis")...); err != nil {
		return resourceRef{}, err
	}

	groupFilter := ""
	versionFilter := ""
	if apiVersion != "" {
		parts := strings.SplitN(apiVersion, "/", 2)
		if len(parts) != 2 {
			return resourceRef{}, fmt.Errorf("%w: apiVersion must be group/version for aggregation APIs", errUsage)
		}
		groupFilter = parts[0]
		versionFilter = parts[1]
	}

	var matches []resourceRef
	for _, group := range groups.Groups {
		if groupFilter != "" && group.Name != groupFilter {
			continue
		}

		for _, version := range group.Versions {
			if versionFilter != "" && version.Version != versionFilter {
				continue
			}

			var resources apiResourceList
			if err := kubectlJSON(kubectl, &resources, append(globalArgs, "get", "--raw", "/apis/"+group.Name+"/"+version.Version)...); err != nil {
				return resourceRef{}, err
			}

			for _, resource := range resources.APIResources {
				if strings.Contains(resource.Name, "/") {
					continue
				}
				if !resourceMatches(token, group.Name, version.Version, resource) {
					continue
				}
				matches = append(matches, resourceRef{
					Group:      group.Name,
					Version:    version.Version,
					Resource:   resource.Name,
					Kind:       resource.Kind,
					Namespaced: resource.Namespaced,
				})
			}
		}
	}

	if len(matches) == 0 {
		if apiVersion != "" {
			return resourceRef{}, fmt.Errorf("resource %q not found in %s", token, apiVersion)
		}
		return resourceRef{}, fmt.Errorf("resource %q not found in aggregated APIs", token)
	}

	if len(matches) == 1 {
		return matches[0], nil
	}

	seen := map[string]bool{}
	unique := make([]string, 0, len(matches))
	for _, match := range matches {
		key := match.Resource + "." + match.Version + "." + match.Group
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, key)
	}
	if len(unique) == 1 {
		return matches[0], nil
	}

	return resourceRef{}, fmt.Errorf("resource %q is ambiguous, use one of: %s", token, strings.Join(unique, ", "))
}

func kubectlJSON(kubectl kubectlRunner, into any, args ...string) error {
	data, err := kubectl.output(args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("failed to decode kubectl JSON response: %w", err)
	}
	return nil
}

func resourceMatches(token, group, version string, resource apiResource) bool {
	token = strings.ToLower(token)
	for _, alias := range resourceAliases(resource, group, version) {
		if token == strings.ToLower(alias) {
			return true
		}
	}
	return false
}

func resourceAliases(resource apiResource, group, version string) []string {
	base := []string{resource.Name}
	if resource.SingularName != "" {
		base = append(base, resource.SingularName)
	}
	if resource.Kind != "" {
		base = append(base, strings.ToLower(resource.Kind))
	}
	base = append(base, resource.ShortNames...)
	base = dedupeStrings(base)

	aliases := make([]string, 0, len(base)*3)
	for _, value := range base {
		aliases = append(aliases, value)
		aliases = append(aliases, value+"."+group)
		aliases = append(aliases, value+"."+version+"."+group)
	}

	return dedupeStrings(aliases)
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func parseManifestMeta(data []byte) (manifestMeta, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var manifest manifestMeta
		err := dec.Decode(&manifest)
		if errors.Is(err, io.EOF) {
			return manifestMeta{}, fmt.Errorf("%w: manifest is empty", errUsage)
		}
		if err != nil {
			return manifestMeta{}, fmt.Errorf("failed to parse manifest: %w", err)
		}

		if manifest.APIVersion == "" && manifest.Kind == "" && manifest.Metadata.Name == "" && manifest.Metadata.Namespace == "" {
			continue
		}
		if manifest.APIVersion == "" || manifest.Kind == "" {
			return manifestMeta{}, fmt.Errorf("%w: manifest must include apiVersion and kind", errUsage)
		}
		return manifest, nil
	}
}

func buildResourcePath(resource resourceRef, namespace string, allNamespaces bool, name, selector, fieldSelector string, watch bool) (string, error) {
	base := "/apis/" + resource.Group + "/" + resource.Version
	if resource.Namespaced {
		if allNamespaces {
			return "", fmt.Errorf("%w: all-namespaces is not supported for namespaced aggregation resources", errUsage)
		}
		if namespace == "" {
			return "", fmt.Errorf("%w: namespace is required for namespaced resource %q", errUsage, resource.Resource)
		}
		base += "/namespaces/" + url.PathEscape(namespace)
	}

	base += "/" + resource.Resource
	if name != "" {
		base += "/" + url.PathEscape(name)
	}

	query := url.Values{}
	if selector != "" {
		query.Set("labelSelector", selector)
	}
	if fieldSelector != "" {
		query.Set("fieldSelector", fieldSelector)
	}
	if watch {
		query.Set("watch", "true")
	}

	if encoded := query.Encode(); encoded != "" {
		base += "?" + encoded
	}

	return base, nil
}

func usage() string {
	return `Usage:
  kubectl harikube get|list|watch <resource>[.<version>.<group>] [name] [flags]
  kubectl harikube create -f FILE [flags]
  kubectl harikube update -f FILE [flags]
  kubectl harikube delete <resource>[.<version>.<group>] [name] [flags]
  kubectl harikube delete -f FILE [flags]

Supported flags:
  -n, --namespace
  -A, --all-namespaces
  -l, --selector
      --field-selector
  -f, --filename
  -o, --output (json|yaml)

Supported verbs:
  get     -> GET aggregation endpoint
  list    -> GET aggregation endpoint
  watch   -> GET aggregation endpoint with watch=true
  create  -> POST aggregation endpoint
  update  -> PUT aggregation endpoint
  delete  -> DELETE aggregation endpoint
`
}
