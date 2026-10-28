package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Kylineyes/outlook-manager/internal/graph"
)

func withUploadProgress(ctx context.Context, disabled bool) context.Context {
	if disabled {
		return ctx
	}
	return graph.WithUploadProgress(ctx, func(progress graph.UploadProgress) {
		if progress.Done {
			fmt.Fprintf(os.Stderr, "\r上传 %s: 100%% (%d/%d 字节)\n", progress.Name, progress.Uploaded, progress.Total)
			return
		}
		fmt.Fprintf(os.Stderr, "\r上传 %s: %.1f%% (%d/%d 字节)", progress.Name, progress.Percent, progress.Uploaded, progress.Total)
	})
}
