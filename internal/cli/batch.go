package cli

// Shared machinery for the batch writers (migrate, apply): a checkpoint
// state file mapping source keys to created issue numbers, throttled writes,
// and label bootstrapping so batch creates never reference a missing label.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/lumberbarons/issues/internal/conventions"
	"github.com/lumberbarons/issues/internal/gh"
)

// ensureLabels creates any missing convention labels plus the given extras.
func (a *App) ensureLabels(ctx context.Context, extras []gh.Label) error {
	existing, err := a.Client.ListLabels(ctx)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, l := range existing {
		have[l.Name] = true
	}
	var want []gh.Label
	for _, l := range conventions.Labels {
		want = append(want, gh.Label{Name: l.Name, Color: l.Color, Description: l.Description})
	}
	want = append(want, extras...)
	for _, l := range want {
		if have[l.Name] {
			continue
		}
		if err := a.Client.CreateLabel(ctx, l); err != nil {
			return fmt.Errorf("creating label %q: %w", l.Name, err)
		}
	}
	return nil
}

// loadBatchState reads a checkpoint file. A missing file is a fresh start; a
// corrupt one aborts — treating it as empty would duplicate every
// already-created issue.
func loadBatchState(path string) (map[string]int, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]int{}, nil
	}
	if err != nil {
		return nil, err
	}
	var state map[string]int
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("%s is not a valid resume-state file: %w", path, err)
	}
	return state, nil
}

func saveBatchState(path string, state map[string]int) error {
	data, err := json.MarshalIndent(state, "", "  ") // map keys marshal sorted
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func sleep(d time.Duration) {
	if d > 0 {
		time.Sleep(d)
	}
}
