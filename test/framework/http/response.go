package http

import (
	"io/ioutil"
	gohttp "net/http"
)

type Response struct {
	Body []byte
}

func buildResponse(resp *gohttp.Response) (Response, error) {
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	return Response{
		Body: body,
	}, nil
}
