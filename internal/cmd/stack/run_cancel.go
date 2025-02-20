package stack

import (
	"context"
	"fmt"

	"github.com/shurcooL/graphql"
	"github.com/spacelift-io/spacectl/internal/cmd/authenticated"
	"github.com/urfave/cli/v2"
)

func runCancel() cli.ActionFunc {
	return func(cliCtx *cli.Context) error {
		stackID, err := getStackID(cliCtx)
		if err != nil {
			return err
		}

		var mutation struct {
			RunDiscard struct {
				ID string `graphql:"id"`
			} `graphql:"runCancel(stack: $stack, run: $run)"`
		}

		variables := map[string]interface{}{
			"stack": graphql.ID(stackID),
			"run":   graphql.ID(cliCtx.String(flagRequiredRun.Name)),
		}

		ctx := context.Background()

		if err := authenticated.Client.Mutate(ctx, &mutation, variables); err != nil {
			return err
		}

		fmt.Println("You have successfully canceled the run")

		fmt.Println("The run can be visited at", authenticated.Client.URL(
			"/stack/%s/run/%s",
			stackID,
			mutation.RunDiscard.ID,
		))

		if !cliCtx.Bool(flagTail.Name) {
			return nil
		}

		terminal, err := runLogsWithAction(ctx, stackID, mutation.RunDiscard.ID, nil)
		if err != nil {
			return err
		}

		return terminal.Error()
	}
}
