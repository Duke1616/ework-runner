package codebook

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/Duke1616/etask/internal/errs"
	codebookSvc "github.com/Duke1616/etask/internal/service/codebook"
	"github.com/ecodeclub/ginx"
)

// Import 接收浏览器选择的文件或目录，并原子导入当前项目。
func (h *Handler) Import(ctx *ginx.Context) (ginx.Result, error) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, (1<<30)+(64<<20))
	if err := ctx.Request.ParseMultipartForm(32 << 20); err != nil {
		result := invalidParameterResult(fmt.Errorf("%w: 解析导入内容失败", errs.ErrInvalidParameter))
		return result, err
	}
	defer func() { _ = ctx.Request.MultipartForm.RemoveAll() }()

	projectID, err := strconv.ParseInt(ctx.PostForm("project_id"), 10, 64)
	if err != nil {
		return invalidProjectIDError, err
	}
	parentID, err := strconv.ParseInt(defaultFormValue(ctx.PostForm("parent_id"), "0"), 10, 64)
	if err != nil {
		return invalidParameterResult(err), err
	}
	var paths []string
	if err = json.Unmarshal([]byte(ctx.PostForm("paths")), &paths); err != nil {
		return invalidParameterResult(err), err
	}
	var overwritePaths []string
	if raw := ctx.PostForm("overwrite_paths"); raw != "" {
		if err = json.Unmarshal([]byte(raw), &overwritePaths); err != nil {
			return invalidParameterResult(err), err
		}
	}
	headers := ctx.Request.MultipartForm.File["files"]
	if len(paths) != len(headers) {
		err = fmt.Errorf("%w: 文件清单和上传内容数量不一致", errs.ErrInvalidParameter)
		return invalidParameterResult(err), err
	}
	files := make([]codebookSvc.ImportFile, 0, len(headers))
	for index, header := range headers {
		fileHeader := header
		files = append(files, codebookSvc.ImportFile{
			Path: paths[index], Size: fileHeader.Size,
			ContentType: fileHeader.Header.Get("Content-Type"),
			Open: func() (io.ReadCloser, error) {
				return fileHeader.Open()
			},
		})
	}
	result, err := h.files.Import(ctx, codebookSvc.ImportRequest{
		ProjectID: projectID, ParentID: parentID, Files: files,
		OverwritePaths: overwritePaths,
	})
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "导入成功", Data: ImportResp{
		FileCount: result.FileCount, DirectoryCount: result.DirectoryCount,
	}}, nil
}

// Download 流式下载项目文件当前版本内容。
func (h *Handler) Download(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return invalidCodebookIDError, err
	}
	file, reader, err := h.files.Open(ctx, id)
	if err != nil {
		return h.translateError(err), err
	}
	defer reader.Close()
	contentType := file.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Name})
	ctx.DataFromReader(http.StatusOK, file.Size, contentType, reader, map[string]string{
		"Content-Disposition": disposition,
	})
	return ginx.Result{}, ginx.ErrNoResponse
}

func defaultFormValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
