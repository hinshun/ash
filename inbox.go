package main

func (api *Api) GetInbox(role string) ([]PullRequest, error) {
	logger.Debug(
		"requesting pull requests count from Stash for role '%s'...",
		role,
	)

	prReply := struct {
		Values []PullRequest
	}{}

	resource := api.GetResource().Res("inbox/latest")
	err := api.DoGet(resource.Res("pull-requests", &prReply),
		map[string]string{
			"limit": "1000",
			"role":  role,
		})

	if err != nil {
		return nil, err
	}

	return prReply.Values, nil
}
