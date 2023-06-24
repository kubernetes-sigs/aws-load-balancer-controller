package http

import (
	gohttp "net/http"
	"os"
)

type Response struct {
	Body         []byte
	ResponseCode int
}

func buildResponse(resp *gohttp.Response) (Response, error) {
	defer resp.Body.Close()
	body, err := os.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	return Response{
		Body:         body,
		ResponseCode: resp.StatusCode,
	}, nil
}
