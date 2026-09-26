package hitkeepcmd

import (
	"context"
	"fmt"
	"io"
	"time"
)

func runUpdateList[T any](
	ctx context.Context,
	outputPath string,
	out, errOut io.Writer,
	fetch func(context.Context) (T, error),
	validate func(T) error,
	save func(string, T) error,
	fetchError, validationError, saveError string,
	write func(io.Writer, T, string),
) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	data, err := fetch(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: %s: %v\n", fetchError, err)
		return &ExitError{Code: 1}
	}
	if err := validate(data); err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: %s: %v\n", validationError, err)
		return &ExitError{Code: 1}
	}
	if err := save(outputPath, data); err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: %s: %v\n", saveError, err)
		return &ExitError{Code: 1}
	}

	write(out, data, outputPath)
	return nil
}
