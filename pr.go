package main

import (
	"fmt"

	"github.com/bndr/gopencils"
	"github.com/seletskiy/godiff"
)

const commentPreviewLen = 40

type unexpectedStatusCode int

func (u unexpectedStatusCode) Error() string {
	return fmt.Sprintf("unexpected status code from Stash: %d", u)
}

type stashApiError []byte

func (s stashApiError) Error() string {
	return string(s)
}

type PullRequest struct {
	*Repo
	Resource *gopencils.Resource

	Id          int64
	Description string
	State       string
	UpdatedDate UnixTimestamp
	ReviewFiles ReviewFiles

	FromRef struct {
		Id         string
		Repository struct {
			Slug    string
			Project struct {
				Key string
			}
		}
	}

	Author struct {
		User struct {
			Name        string
			DisplayName string
		}
	}

	Reviewers []struct {
		Approved bool
		User     struct {
			Name string
		}
	}

	Properties struct {
		CommentCount int64
	}
}

type PullRequestInfo struct {
	Version int64
	Links   struct {
		Self []struct {
			Href string
		}
	}
}

func (pr *PullRequest) GetInfo() (*PullRequestInfo, error) {
	pr.Resource.Response = &PullRequestInfo{}
	err := pr.DoGet(pr.Resource)
	if err != nil {
		return nil, err
	}

	return pr.Resource.Response.(*PullRequestInfo), nil
}

func (pr *PullRequest) GetReview(
	path string, ignoreWhitespaces bool,
) (*Review, error) {
	result := godiff.Changeset{}

	queryString := make(map[string]string)
	if ignoreWhitespaces {
		queryString["whitespace"] = "ignore-all"
	}

	err := pr.DoGet(
		pr.Resource.Res("diff").Id(path, &result).SetQuery(queryString),
	)
	if err != nil {
		return nil, err
	}

	for _, diff := range result.Diffs {
		diff.Attributes.FromHash = []string{result.FromHash}
		diff.Attributes.ToHash = []string{result.ToHash}
	}

	// TODO: refactor block
	result.ForEachLine(
		func(
			diff *godiff.Diff, _ *godiff.Hunk,
			_ *godiff.Segment, line *godiff.Line,
		) error {
			for _, id := range line.CommentIds {
				for _, c := range diff.LineComments {
					if c.Id == id {
						line.Comments = append(line.Comments, c)
						diff.LineComments = append(diff.LineComments, c)
						break
					}
				}
			}

			return nil
		})

	result.Path = path

	logger.Debug("successfully got review from Stash")

	return &Review{
		changeset:  result,
		isOverview: false,
	}, nil
}

func (pr *PullRequest) Approve() error {
	resource := make(map[string]interface{})
	return pr.DoPost(pr.Resource.Res("approve", &resource))
}

func (pr *PullRequest) Decline() error {
	info, err := pr.GetInfo()
	if err != nil {
		return err
	}

	query := map[string]string{
		"version": fmt.Sprint(info.Version),
	}

	resource := make(map[string]interface{})

	return pr.DoPost(pr.Resource.Res("decline", &resource).SetQuery(query))
}

func (pr *PullRequest) Merge() error {
	info, err := pr.GetInfo()
	if err != nil {
		return err
	}

	query := map[string]string{
		"version": fmt.Sprint(info.Version),
	}

	resource := make(map[string]interface{})

	return pr.DoPost(pr.Resource.Res("merge", &resource).SetQuery(query))
}

func (pr *PullRequest) GetActivities(limit string) (*Review, error) {
	query := map[string]string{
		"limit": limit,
	}

	response := struct {
		Value ReviewActivity `json:"values"`
	}{}

	err := pr.DoGet(pr.Resource.Res("activities", &response), query)
	if err != nil {
		return nil, err
	}

	logger.Debug("successfully got review from Stash")

	return &Review{
		changeset: godiff.Changeset{
			Diffs: response.Value.Changeset.Diffs,
		},
		isOverview: true,
	}, nil
}

func (pr *PullRequest) GetFiles() (ReviewFiles, error) {
	if len(pr.ReviewFiles) > 0 {
		return pr.ReviewFiles, nil
	}

	pr.ReviewFiles = make(ReviewFiles, 0)

	query := map[string]string{
		"start": "0",
		"limit": "1000",
	}

	err := pr.DoGet(pr.Resource.Res("changes", &pr.ReviewFiles), query)
	if err != nil {
		return nil, err
	}

	logger.Debug("successfully got files list from Stash")

	return pr.ReviewFiles, nil
}

func (pr *PullRequest) ApplyChange(change ReviewChange) error {
	switch c := change.(type) {
	case ReplyAdded:
		logger.Info("replying to <%d>: <%s>", c.parent.Id,
			c.comment.Short(commentPreviewLen))
		return pr.addComment(c)
	case LineCommentAdded:
		logger.Info("commenting (L%d): <%s>",
			c.comment.Anchor.Line,
			c.comment.Short(commentPreviewLen))
		return pr.addComment(c)
	case CommentRemoved:
		logger.Info("wasting comment: <%d>",
			c.comment.Id)
		return pr.removeComment(c)
	case CommentModified:
		logger.Info("modifying comment <%d>: <%s>",
			c.comment.Id, c.comment.Short(commentPreviewLen))
		return pr.modifyComment(c)
	case ReviewCommentAdded:
		logger.Info("adding review level comment: <%s>",
			c.comment.Short(commentPreviewLen))
		return pr.addComment(c)
	case FileCommentAdded:
		logger.Info("adding file level comment: <%s>",
			c.comment.Short(commentPreviewLen))
		return pr.addComment(c)
	default:
		logger.Warning("unexpected <change> argument: %#v", change)
	}

	return nil
}

func (pr *PullRequest) addComment(change ReviewChange) error {
	result := godiff.Comment{}

	err := pr.DoPost(pr.Resource.Res("comments", &result), change.GetPayload())
	if err != nil {
		return err
	}

	logger.Info("comment added: <%d>", result.Id)

	return nil
}

func (pr *PullRequest) modifyComment(change CommentModified) error {
	query := map[string]string{
		"version": fmt.Sprint(change.comment.Version),
	}
	result := godiff.Comment{}

	err := pr.DoPut(
		pr.Resource.
			Res("comments").
			Id(fmt.Sprint(change.comment.Id), &result).
			SetQuery(query),
		change.GetPayload())
	if err != nil {
		return err
	}

	logger.Info("comment modified: <%d>, version %d", result.Id, result.Version)

	return nil
}

func (pr *PullRequest) removeComment(change CommentRemoved) error {
	query := map[string]string{
		"version": fmt.Sprint(change.comment.Version),
	}

	result := make(map[string]interface{})

	logger.Debug("accessing Stash...")

	req := pr.Resource.
		Res("comments").
		Id(fmt.Sprint(change.comment.Id), &result).
		SetQuery(query)

	err := pr.DoDelete(req)
	if err != nil && req.Raw.StatusCode != 204 {
		return err
	}

	logger.Info("comment wasted: <%d>", change.comment.Id)

	return nil
}
