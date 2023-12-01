package http

import (
	"io"
	gohttp "net/http"
)

type Response struct {
	Body         []byte
	ResponseCode int
}

func buildResponse(resp *gohttp.Response) (Response, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	return Response{
		Body:         body,
		ResponseCode: resp.StatusCode,
	}, nil
}
