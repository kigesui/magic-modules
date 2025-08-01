package cmd

import (
	"fmt"
	"magician/exec"
	"magician/provider"
	"magician/source"
	"magician/vcr"
	"os"

	"github.com/spf13/cobra"
)

var ccRequiredEnvironmentVariables = [...]string{
	"COMMIT_SHA",
	"GOCACHE",
	"GOPATH",
	"GOOGLE_BILLING_ACCOUNT",
	"GOOGLE_CUST_ID",
	"GOOGLE_IDENTITY_USER",
	"GOOGLE_MASTER_BILLING_ACCOUNT",
	"GOOGLE_ORG",
	"GOOGLE_ORG_2",
	"GOOGLE_ORG_DOMAIN",
	"GOOGLE_PROJECT",
	"GOOGLE_PROJECT_NUMBER",
	"GOOGLE_REGION",
	"GOOGLE_SERVICE_ACCOUNT",
	"GOOGLE_PUBLIC_AVERTISED_PREFIX_DESCRIPTION",
	"GOOGLE_ZONE",
	"PATH",
	"SA_KEY",
}

var ccOptionalEnvironmentVariables = [...]string{
	"GOOGLE_CHRONICLE_INSTANCE_ID",
	"GOOGLE_VMWAREENGINE_PROJECT",
}

var checkCassettesCmd = &cobra.Command{
	Use:   "check-cassettes",
	Short: "Run VCR tests on downstream main branch",
	Long: `This command runs after downstream changes are merged and runs the most recent
	VCR cassettes using the newly built beta provider.

	The following environment variables are required:
` + listCCRequiredEnvironmentVariables() + `

	It prints a list of tests that failed in replaying mode along with all test output.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		env := make(map[string]string)
		for _, ev := range ccRequiredEnvironmentVariables {
			val, ok := os.LookupEnv(ev)
			if !ok {
				return fmt.Errorf("did not provide %s environment variable", ev)
			}
			env[ev] = val
		}
		for _, ev := range ccOptionalEnvironmentVariables {
			val, ok := os.LookupEnv(ev)
			if ok {
				env[ev] = val
			} else {
				fmt.Printf("🟡 Did not provide %s environment variable\n", ev)
			}
		}

		githubToken, ok := lookupGithubTokenOrFallback("GITHUB_TOKEN_DOWNSTREAMS")
		if !ok {
			return fmt.Errorf("did not provide GITHUB_TOKEN_DOWNSTREAMS or GITHUB_TOKEN environment variables")
		}

		rnr, err := exec.NewRunner()
		if err != nil {
			return fmt.Errorf("error creating Runner: %w", err)
		}

		ctlr := source.NewController(env["GOPATH"], "modular-magician", githubToken, rnr)

		vt, err := vcr.NewTester(env, "ci-vcr-cassettes", "vcr-check-cassettes", rnr)
		if err != nil {
			return fmt.Errorf("error creating VCR tester: %w", err)
		}
		return execCheckCassettes(env["COMMIT_SHA"], vt, ctlr)
	},
}

func lookupGithubTokenOrFallback(tokenName string) (string, bool) {
	val, ok := os.LookupEnv(tokenName)
	if !ok {
		return os.LookupEnv("GITHUB_TOKEN")
	}
	return val, ok
}

func listCCRequiredEnvironmentVariables() string {
	var result string
	for i, ev := range ccRequiredEnvironmentVariables {
		result += fmt.Sprintf("\t%2d. %s\n", i+1, ev)
	}
	return result
}

func execCheckCassettes(commit string, vt *vcr.Tester, ctlr *source.Controller) error {
	if err := vt.FetchCassettes(provider.Beta, "main", ""); err != nil {
		return fmt.Errorf("error fetching cassettes: %w", err)
	}

	providerRepo := &source.Repo{
		Name:   provider.Beta.RepoName(),
		Branch: "downstream-pr-" + commit,
	}
	ctlr.SetPath(providerRepo)
	if err := ctlr.Clone(providerRepo); err != nil {
		return fmt.Errorf("error cloning provider: %w", err)
	}
	vt.SetRepoPath(provider.Beta, providerRepo.Path)

	result, err := vt.Run(vcr.RunOptions{
		Mode:    vcr.Replaying,
		Version: provider.Beta,
	})
	if err != nil {
		fmt.Println("Error running VCR: ", err)
	}
	if err := vt.UploadLogs(vcr.UploadLogsOptions{
		Mode:    vcr.Replaying,
		Version: provider.Beta,
	}); err != nil {
		return fmt.Errorf("error uploading logs: %w", err)
	}
	fmt.Println(len(result.FailedTests), " failed tests: ", result.FailedTests)
	// TODO report these failures to bigquery
	fmt.Println(len(result.PassedTests), " passed tests: ", result.PassedTests)
	fmt.Println(len(result.SkippedTests), " skipped tests: ", result.SkippedTests)

	if err := vt.Cleanup(); err != nil {
		return fmt.Errorf("error cleaning up vcr tester: %w", err)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(checkCassettesCmd)
}
