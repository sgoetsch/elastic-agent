// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/elastic/elastic-agent-libs/logp"
	"github.com/elastic/elastic-agent/internal/pkg/agent/application/filelock"
	"github.com/elastic/elastic-agent/internal/pkg/agent/application/paths"
	"github.com/elastic/elastic-agent/internal/pkg/agent/install"
	"github.com/elastic/elastic-agent/internal/pkg/cli"
	"github.com/elastic/elastic-agent/pkg/core/logger"
	"github.com/elastic/elastic-agent/pkg/utils"
)

const (
	flagInstallBasePath               = "base-path"
	flagInstallUnprivileged           = "unprivileged"
	flagInstallDevelopment            = "develop"
	flagInstallNamespace              = "namespace"
	flagInstallRunUninstallFromBinary = "run-uninstall-from-binary"
	flagInstallServers                = "install-servers"

	flagInstallCustomUser  = "user"
	flagInstallCustomGroup = "group"
	flagInstallCustomPass  = "password"
)

func newInstallCommandWithArgs(_ []string, streams *cli.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Elastic Agent permanently on this system",
		Long: `This command installs Elastic Agent permanently on this system. The system's service manager then manages the installed Elastic agent.

Unless all the require command-line parameters are provided or -f is used this command will ask questions on how you
would like the Agent to operate.
`,
		Run: func(c *cobra.Command, _ []string) {
			if err := installCmd(streams, c); err != nil {
				fmt.Fprintf(streams.Err, "Error: %v\n%s\n", err, troubleshootMessage())
				logExternal(fmt.Sprintf("%s install failed: %s", paths.BinaryName, err))
				os.Exit(1)
			}
		},
	}

	cmd.Flags().BoolP("force", "f", false, "Force overwrite the current installation and do not prompt for confirmation")
	cmd.Flags().BoolP("non-interactive", "n", false, "Install Elastic Agent in non-interactive mode which will not prompt on missing parameters but fails instead.")
	cmd.Flags().String(flagInstallBasePath, paths.DefaultBasePath, "The path where the Elastic Agent will be installed. It must be an absolute path.")
	cmd.Flags().Bool(flagInstallUnprivileged, false, "Install in unprivileged mode, limiting the access of the Elastic Agent.")
	cmd.Flags().Bool(flagInstallServers, false, "Install larger version of agent that includes server components")

	cmd.Flags().Bool(flagInstallRunUninstallFromBinary, false, "Run the uninstall command from this binary instead of using the binary found in the system's path.")
	_ = cmd.Flags().MarkHidden(flagInstallRunUninstallFromBinary) // Advanced option to force a new agent to override an existing installation, it may orphan installed components.

	cmd.Flags().String(flagInstallNamespace, "", "Install into an isolated namespace. Allows multiple Elastic Agents to be installed at once. (experimental)")
	_ = cmd.Flags().MarkHidden(flagInstallNamespace) // For internal use only.

	cmd.Flags().Bool(flagInstallDevelopment, false, "Install into a standardized development namespace, may enable development specific options. Allows multiple Elastic Agents to be installed at once. (experimental)")
	_ = cmd.Flags().MarkHidden(flagInstallDevelopment) // For internal use only.

	// Active directory user specification
	cmd.Flags().String(flagInstallCustomUser, "", "Custom user used to run Elastic Agent")
	cmd.Flags().String(flagInstallCustomGroup, "", "Custom group used to access Elastic Agent files")
	if runtime.GOOS == "windows" {
		cmd.Flags().String(flagInstallCustomPass, "", "Password for user used to run Elastic Agent")
	}

	addEnrollFlags(cmd)

	return cmd
}

func installCmd(streams *cli.IOStreams, cmd *cobra.Command) error {
	var err error

	if installServers, _ := cmd.Flags().GetBool(flagInstallServers); isFleetServerFlagProvided(cmd) && !installServers {
		_ = cmd.Flags().Lookup(flagInstallServers).Value.Set("true") // this can fail only when parsing bool
		fmt.Fprintf(streams.Out, "fleet-server installation detected, using --%s flag\n", flagInstallServers)
	}

	err = validateEnrollFlags(cmd)
	if err != nil {
		return fmt.Errorf("could not validate flags: %w", err)
	}

	basePath, _ := cmd.Flags().GetString(flagInstallBasePath)
	if !filepath.IsAbs(basePath) {
		return fmt.Errorf("base path [%s] is not absolute", basePath)
	}

	isAdmin, err := utils.HasRoot()
	if err != nil {
		return fmt.Errorf("unable to perform install command while checking for root/Administrator rights: %w", err)
	}
	if !isAdmin {
		return fmt.Errorf("unable to perform install command, not executed with %s permissions", utils.PermissionUser)
	}

	unprivileged, _ := cmd.Flags().GetBool(flagInstallUnprivileged)
	if unprivileged {
		fmt.Fprintln(streams.Out, "Unprivileged installation mode enabled.")
	}

	isDevelopmentMode, _ := cmd.Flags().GetBool(flagInstallDevelopment)
	if isDevelopmentMode {
		fmt.Fprintln(streams.Out, "Installing into development namespace; this is an experimental and currently unsupported feature.")
		// For now, development mode only installs agent in a well known namespace to allow two agents on the same machine.
		paths.SetInstallNamespace(paths.DevelopmentNamespace)
	}

	namespace, _ := cmd.Flags().GetString(flagInstallNamespace)
	if namespace != "" {
		fmt.Fprintf(streams.Out, "Installing into namespace '%s'; this is an experimental and currently unsupported feature.\n", namespace)
		// Overrides the development namespace if namespace was specified separately.
		paths.SetInstallNamespace(namespace)
	}

	topPath := paths.InstallPath(basePath)

	status, reason := install.Status(topPath)
	force, _ := cmd.Flags().GetBool("force")
	if status == install.Installed && !force {
		return fmt.Errorf("already installed at: %s", topPath)
	}

	runUninstallBinary, _ := cmd.Flags().GetBool(flagInstallRunUninstallFromBinary)
	if status == install.Installed && force && runUninstallBinary {
		fmt.Fprintln(streams.Out, "Uninstall will not be ran from the agent installed in system path, components may persist.")
	}

	nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
	if nonInteractive {
		fmt.Fprintln(streams.Out, "Installing in non-interactive mode.")
	}

	if status == install.PackageInstall {
		fmt.Fprintf(streams.Out, "Installed as a system package, installation will not be altered.\n")
	}

	// check the lock to ensure that elastic-agent is not already running in this directory
	locker := filelock.NewAppLocker(paths.Data(), paths.AgentLockFileName)
	if err := locker.TryLock(); err != nil {
		if errors.Is(err, filelock.ErrAppAlreadyRunning) {
			return fmt.Errorf("cannot perform installation as Elastic Agent is already running from this directory")
		}
		return fmt.Errorf("error obtaining lock: %w", err)
	}
	_ = locker.Unlock()

	if status == install.Broken {
		if !force && !nonInteractive {
			fmt.Fprintf(streams.Out, "Elastic Agent is installed but currently broken: %s\n", reason)
			confirm, err := cli.Confirm(fmt.Sprintf("Continuing will re-install Elastic Agent over the current installation at %s. Do you want to continue?", topPath), true)
			if err != nil {
				return fmt.Errorf("problem reading prompt response")
			}
			if !confirm {
				return fmt.Errorf("installation was cancelled by the user")
			}
		}
	} else if status != install.PackageInstall {
		if !force && !nonInteractive {
			confirm, err := cli.Confirm(fmt.Sprintf("Elastic Agent will be installed at %s and will run as a service. Do you want to continue?", topPath), true)
			if err != nil {
				return fmt.Errorf("problem reading prompt response")
			}
			if !confirm {
				return fmt.Errorf("installation was cancelled by the user")
			}
		}
	}

	enroll := true
	askEnroll := true
	url, _ := cmd.Flags().GetString("url")
	token, _ := cmd.Flags().GetString("enrollment-token")
	delayEnroll, _ := cmd.Flags().GetBool("delay-enroll")
	if url != "" && token != "" {
		askEnroll = false
	}
	fleetServer, _ := cmd.Flags().GetString("fleet-server-es")
	if fleetServer != "" || force || delayEnroll || nonInteractive {
		askEnroll = false
	}
	if askEnroll {
		confirm, err := cli.Confirm("Do you want to enroll this Agent into Fleet?", true)
		if err != nil {
			return fmt.Errorf("problem reading prompt response")
		}
		if !confirm {
			// not enrolling
			enroll = false
		}
	}
	if !askEnroll && (url == "" || token == "") && fleetServer == "" {
		// force was performed without required enrollment arguments, all done (standalone mode)
		enroll = false
	}

	if enroll && fleetServer == "" {
		if url == "" {
			if nonInteractive {
				return fmt.Errorf("missing required --url argument used to enroll the agent")
			}
			url, err = cli.ReadInput("URL you want to enroll this Agent into:")
			if err != nil {
				return fmt.Errorf("problem reading prompt response")
			}
			if url == "" {
				fmt.Fprintln(streams.Out, "Enrollment cancelled because no URL was provided.")
				return nil
			}
		}
		if token == "" {
			if nonInteractive {
				return fmt.Errorf("missing required --enrollment-token argument used to enroll the agent")
			}
			token, err = cli.ReadInput("Fleet enrollment token:")
			if err != nil {
				return fmt.Errorf("problem reading prompt response")
			}
			if token == "" {
				fmt.Fprintf(streams.Out, "Enrollment cancelled because no enrollment token was provided.\n")
				return nil
			}
		}
	}

	progBar := install.CreateAndStartNewSpinner(streams.Out, "Installing Elastic Agent...")

	log, logBuff := logger.NewInMemory("install", logp.ConsoleEncoderConfig())
	defer func() {
		if err == nil {
			return
		}
		fmt.Fprintf(os.Stderr, "Error uninstalling. Printing logs\n")
		fmt.Fprint(os.Stderr, logBuff.String())
	}()

	var ownership utils.FileOwner
	cfgFile := paths.ConfigFile()
	if status == install.Installed {
		// Uninstall the agent
		progBar.Describe(fmt.Sprintf("Uninstalling current %s", paths.ServiceDisplayName()))
		if !runUninstallBinary {
			err := execUninstall(streams, topPath, paths.BinaryName)
			if err != nil {
				progBar.Describe("Uninstall failed")
				return err
			}
		} else {
			err := install.Uninstall(cmd.Context(), cfgFile, topPath, "", log, progBar, false)
			if err != nil {
				progBar.Describe("Uninstall from binary failed")
				return err
			}
		}
		progBar.Describe("Successfully uninstalled Elastic Agent")
	}

	if status != install.PackageInstall {
		customUser, _ := cmd.Flags().GetString(flagInstallCustomUser)
		customGroup, _ := cmd.Flags().GetString(flagInstallCustomGroup)
		customPass := ""
		if runtime.GOOS == "windows" {
			customPass, _ = cmd.Flags().GetString(flagInstallCustomPass)
		}

		flavor := install.DefaultFlavor
		if installServers, _ := cmd.Flags().GetBool(flagInstallServers); installServers {
			flavor = install.FlavorServers
		}

		ownership, err = install.Install(cfgFile, topPath, unprivileged, log, progBar, streams, customUser, customGroup, customPass, flavor)
		if err != nil {
			return fmt.Errorf("error installing package: %w", err)
		}

		defer func() {
			if err != nil {
				progBar.Describe("Uninstalling")
				innerErr := install.Uninstall(cmd.Context(), cfgFile, topPath, "", log, progBar, false)
				if innerErr != nil {
					progBar.Describe("Failed to Uninstall")
				} else {
					progBar.Describe("Uninstalled")
				}
			}
		}()

		if !delayEnroll {
			progBar.Describe("Starting Service")
			err = install.StartService(topPath)
			if err != nil {
				progBar.Describe("Start Service failed, exiting...")
				fmt.Fprintf(streams.Out, "Installation failed to start '%s' service.\n", paths.ServiceName())
				return fmt.Errorf("error starting service: %w", err)
			}
			progBar.Describe("Service Started")

			defer func() {
				if err != nil {
					progBar.Describe("Stopping Service")
					innerErr := install.StopService(topPath, install.DefaultStopTimeout, install.DefaultStopInterval)
					if innerErr != nil {
						progBar.Describe("Failed to Stop Service")
					} else {
						progBar.Describe("Successfully Stopped Service")
					}
				}
			}()
		}

		fmt.Fprintf(streams.Out, "%s successfully installed, starting enrollment.\n", paths.ServiceDisplayName())
	}

	if enroll {
		enrollArgs := []string{"enroll", "--from-install"}
		enrollArgs = append(enrollArgs, buildEnrollmentFlags(cmd, url, token)...)
		enrollCmd := exec.Command(install.ExecutablePath(topPath), enrollArgs...) //nolint:gosec // it's not tainted
		enrollCmd.Stdin = os.Stdin
		enrollCmd.Stdout = os.Stdout
		enrollCmd.Stderr = os.Stderr
		err = enrollCmdExtras(enrollCmd, ownership)
		if err != nil {
			return err
		}

		progBar.Describe(fmt.Sprintf("Enrolling %s with Fleet", paths.ServiceDisplayName()))
		err = enrollCmd.Start()
		if err != nil {
			progBar.Describe("Failed to Enroll")
			return fmt.Errorf("failed to execute enroll command: %w", err)
		}
		progBar.Describe("Waiting For Enroll...")
		err = enrollCmd.Wait()
		if err != nil {
			progBar.Describe("Failed to Enroll")
			// uninstall doesn't need to be performed here the defer above will
			// catch the error and perform the uninstall
			return fmt.Errorf("enroll command failed for unknown reason: %w", err)
		}
		progBar.Describe("Enroll Completed")
	}

	progBar.Describe("Done")
	_ = progBar.Finish()
	_ = progBar.Exit()
	fmt.Fprintf(streams.Out, "\n%s has been successfully installed.\n", paths.ServiceDisplayName())
	return nil
}

func isFleetServerFlagProvided(cmd *cobra.Command) bool {
	var fleetServerFlagPresent bool
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if fleetServerFlagPresent {
			return
		}

		if !strings.HasPrefix(f.Name, "fleet-server-") {
			return
		}

		flag := cmd.Flags().Lookup(f.Name)
		if flag != nil && flag.Changed {
			fleetServerFlagPresent = true
		}

	})

	return fleetServerFlagPresent
}

// execUninstall execs "elastic-agent uninstall --force" from the elastic agent installed on the system (found in PATH)
func execUninstall(streams *cli.IOStreams, topPath string, binName string) error {
	args := []string{
		"uninstall",
		"--force",
	}

	// Using the topPath with binaryName is feasible only because the shell wrapper (linux) does not
	// do anything complicated aside from calling the agent binary. If this were
	// to change, the implementation here may need to change as well.
	binPath := filepath.Join(topPath, binName)
	fi, err := os.Stat(binPath)
	if err != nil {
		return fmt.Errorf("error checking binary path %s: %w", binPath, err)
	}

	if fi.IsDir() {
		return fmt.Errorf("expected file, found a directory at %s", binPath)
	}

	uninstall := exec.Command(binPath, args...)
	uninstall.Stdout = streams.Out
	uninstall.Stderr = streams.Err
	if err := uninstall.Start(); err != nil {
		return fmt.Errorf("unable to start elastic-agent uninstall: %w", err)
	}
	if err := uninstall.Wait(); err != nil {
		return fmt.Errorf("failed to uninstall elastic-agent: %w", err)
	}
	return nil
}
