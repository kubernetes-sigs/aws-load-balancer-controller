package main

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/ticketmaster/aws-sdk-go-cache/cache"
)

const pageSize = 10

func main() {
	s := session.Must(session.NewSession())

	// Adds caching to session
	cacheCfg := cache.NewConfig(0 * time.Second)
	cache.AddCaching(s, cacheCfg)

	// Set a custom TTL for ec2 DescribeTags
	cacheCfg.SetCacheTTL("ec2", "DescribeTags", 30*time.Second)

	// Add a handler to print the cache status and how long the request took
	s.Handlers.Complete.PushFront(func(r *request.Request) {
		ctx := r.HTTPRequest.Context()
		fmt.Printf("cached [%v] service [%s.%s]: ",
			cache.IsCacheHit(ctx),
			r.ClientInfo.ServiceName,
			r.Operation.Name,
		)
	})

	svc := ec2.New(s)

	fmt.Println("First Pass")
	pageNum := 0
	err := svc.DescribeTagsPages(&ec2.DescribeTagsInput{MaxResults: aws.Int64(pageSize)},
		func(page *ec2.DescribeTagsOutput, lastPage bool) bool {
			pageNum++
			fmt.Printf("Page %v returned %v tags.\n", pageNum, len(page.Tags))
			return pageNum <= 3
		})
	if err != nil {
		panic(err)
	}

	fmt.Println()
	fmt.Println("Second Pass")
	pageNum = 0
	err = svc.DescribeTagsPages(&ec2.DescribeTagsInput{MaxResults: aws.Int64(pageSize)},
		func(page *ec2.DescribeTagsOutput, lastPage bool) bool {
			pageNum++
			fmt.Printf("Page %v returned %v tags.\n", pageNum, len(page.Tags))
			return pageNum <= 3
		})
	if err != nil {
		panic(err)
	}

	fmt.Println()
	fmt.Println("Third Pass")
	pageNum = 0
	err = svc.DescribeTagsPages(&ec2.DescribeTagsInput{MaxResults: aws.Int64(pageSize)},
		func(page *ec2.DescribeTagsOutput, lastPage bool) bool {
			pageNum++
			fmt.Printf("Page %v returned %v tags.\n", pageNum, len(page.Tags))
			return pageNum <= 3
		})
	if err != nil {
		panic(err)
	}
}
