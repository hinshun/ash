package main

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/bndr/gopencils"
)

type Api struct {
	URL   string
	User  string
	Token string
}

type Project struct {
	*Api
	Name string
}

type ApiError struct {
	Errors []struct {
		Message string
	}
}

type UnixTimestamp int

func (u UnixTimestamp) String() string {
	return u.AsTime().Format("Mon Jan _2 15:04 2006")
}

func (u UnixTimestamp) AsTime() time.Time {
	return time.Unix(int64(u/1000), 0)
}

func (api Api) GetResource() *gopencils.Resource {
	return gopencils.Api(fmt.Sprintf("%s/rest", api.URL))
}

func (api Api) DoGet(
	res *gopencils.Resource,
	payload ...interface{},
) error {
	logger.Debug("performing GET %s %v", res.Url, payload)
	return api.doRequest(res,
		func() (*gopencils.Resource, error) { return res.Get(payload...) })
}

func (api Api) DoPost(
	res *gopencils.Resource,
	payload ...interface{},
) error {
	logger.Debug("performing POST %s %v", res.Url, payload)
	return api.doRequest(res,
		func() (*gopencils.Resource, error) { return res.Post(payload...) })
}

func (api Api) DoPut(
	res *gopencils.Resource,
	payload ...interface{},
) error {
	logger.Debug("performing PUT %s %v", res.Url, payload)
	return api.doRequest(res,
		func() (*gopencils.Resource, error) { return res.Put(payload...) })
}

func (api Api) DoDelete(
	res *gopencils.Resource,
	payload ...interface{},
) error {
	logger.Debug("performing DELETE %s %v", res.Url, payload)
	return api.doRequest(res,
		func() (*gopencils.Resource, error) { return res.Delete(payload...) })
}

func (api Api) doRequest(
	res *gopencils.Resource,
	doFunc func() (*gopencils.Resource, error),
) error {
	res.SetHeader("X-Auth-User", api.User)
	res.SetHeader("X-Auth-Token", api.Token)
	resp, err := doFunc()
	if err != nil {
		return err
	}

	if err := checkErrorStatus(resp); err != nil {
		logger.Warningf("Stash returned error code: %d", resp.Raw.StatusCode)
		return err
	} else {
		logger.Debugf("Stash returned status code: %d", resp.Raw.StatusCode)
	}

	return nil
}

func (project Project) GetRepo(name string) Repo {
	return Repo{
		Project: &project,
		Name:    name,
		Resource: project.GetResource().
			Res("api/1.0").Res(project.Name).Res("repos").Res(name),
	}
}

func checkErrorStatus(resp *gopencils.Resource) error {
	switch resp.Raw.StatusCode {
	case 200, 201, 204:
		return nil

	case 400, 401, 404, 409:
		errorBody, _ := ioutil.ReadAll(resp.Raw.Body)
		if len(errorBody) > 0 {
			return stashApiError(errorBody)
		} else {
			return unexpectedStatusCode(resp.Raw.StatusCode)
		}

	default:
		return unexpectedStatusCode(resp.Raw.StatusCode)
	}
}
