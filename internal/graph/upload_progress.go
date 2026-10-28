package graph

import "context"

// UploadProgress 表示大附件上传进度。
type UploadProgress struct {
	Name     string  `json:"name"`
	Uploaded int64   `json:"uploaded"`
	Total    int64   `json:"total"`
	Percent  float64 `json:"percent"`
	Done     bool    `json:"done"`
}

type uploadProgressKey struct{}

// WithUploadProgress 给 context 添加附件上传进度回调。
func WithUploadProgress(ctx context.Context, callback func(UploadProgress)) context.Context {
	if callback == nil {
		return ctx
	}
	return context.WithValue(ctx, uploadProgressKey{}, callback)
}

func reportUploadProgress(ctx context.Context, progress UploadProgress) {
	callback, ok := ctx.Value(uploadProgressKey{}).(func(UploadProgress))
	if !ok || callback == nil {
		return
	}
	if progress.Total > 0 {
		progress.Percent = float64(progress.Uploaded) / float64(progress.Total) * 100
	}
	callback(progress)
}
