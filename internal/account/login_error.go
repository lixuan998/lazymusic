package account

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ErrHelperRuntime identifies a C++ ABI gap, which an isolated runtime can repair.
var ErrHelperRuntime = errors.New("Apple sign-in needs a compatible C++ runtime")

var helperABI = regexp.MustCompile(`\b(?:GLIBCXX|CXXABI|GLIBC)_[0-9]+(?:\.[0-9]+){1,3}\b`)
var helperLibrary = regexp.MustCompile(`loading shared libraries: ([A-Za-z0-9_.+-]{1,100}\.so(?:\.[0-9]+)*):`)
var brokerStatus = regexp.MustCompile(`token broker returned ([1-5][0-9]{2})\b`)

// helperProblem returns only predefined text and constrained library/version
// names. Helper output may contain credentials, so it must never become an error.
func helperProblem(line string) error {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "not found") && helperABI.MatchString(line):
		version := helperABI.FindString(line)
		if strings.HasPrefix(version, "GLIBCXX_") || strings.HasPrefix(version, "CXXABI_") {
			return fmt.Errorf("%w (%s missing)", ErrHelperRuntime, version)
		}
		return fmt.Errorf("Apple sign-in component requires %s; use a compatible Linux distribution", version)
	case helperLibrary.MatchString(line):
		return fmt.Errorf("Apple sign-in component is missing %s; install the Linux runtime dependencies in README", helperLibrary.FindStringSubmatch(line)[1])
	case strings.Contains(lower, "address already in use"):
		return errors.New("Apple sign-in port is already in use; close the other sign-in attempt and retry")
	case strings.Contains(lower, "x509:") || strings.Contains(lower, "certificate verify failed"):
		return errors.New("Apple sign-in certificate verification failed; check ca-certificates, system time and proxy certificates")
	case strings.Contains(lower, "no such host") || strings.Contains(lower, "temporary failure in name resolution"):
		return errors.New("Apple sign-in service DNS lookup failed; check the WSL network and DNS settings")
	case strings.Contains(lower, "context deadline exceeded") || strings.Contains(lower, "timed out") || strings.Contains(lower, "timeout"):
		return errors.New("Apple sign-in timed out; check the network/proxy and retry")
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "network is unreachable") || strings.Contains(lower, "connection reset"):
		return errors.New("Apple sign-in service could not be reached; check the network and proxy settings")
	case brokerStatus.MatchString(line):
		return fmt.Errorf("Apple sign-in token service returned HTTP %s; retry later", brokerStatus.FindStringSubmatch(line)[1])
	case strings.Contains(lower, "fetching developer token from broker"):
		return errors.New("Apple sign-in token service failed before the login page was ready; check the network/proxy and retry")
	}
	return nil
}

// CheckLoginHelper exercises only version, before starting an auth flow.
func CheckLoginHelper(ctx context.Context, helper, libraryDir string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, helper, "version", "--no-update", "--no-discord")
	setHelperLibrary(cmd, libraryDir)
	output := &loginURLWriter{}
	cmd.Stdout, cmd.Stderr = output, output
	err := cmd.Run()
	output.flush()
	return helperFailure(ctx, err, output.problem)
}

func setHelperLibrary(cmd *exec.Cmd, dir string) {
	if dir != "" {
		// The loader option applies to this executable only. Exporting
		// LD_LIBRARY_PATH would also alter the browser launched by the helper.
		args := []string{"/lib64/ld-linux-x86-64.so.2", "--library-path", dir, cmd.Path}
		cmd.Path, cmd.Args = args[0], append(args, cmd.Args[1:]...)
	}
}

func helperFailure(ctx context.Context, err, problem error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if problem != nil {
		return problem
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return fmt.Errorf("Apple sign-in component exited with code %d; check network and Linux runtime dependencies, then retry", exit.ExitCode())
	}
	return errors.New("could not start Apple sign-in component; check executable permissions and Linux runtime dependencies")
}
