package devtool

import (
	"bufio"
	"errors"
	"os"

	"hitkeep/internal/jsonapi"
)

const (
	maxDevEventSegmentBytes = 10 << 20
	maxDevEventSegmentCount = 10_000
)

func (sink *devEventSink) rotateIfNeeded(nextBytes int) error {
	if sink.segmentBytes+int64(nextBytes) <= maxDevEventSegmentBytes && sink.segmentCount < maxDevEventSegmentCount {
		return nil
	}
	if err := sink.file.Close(); err != nil {
		return err
	}
	active := sink.app.devEventsPath(sink.record.GenerationID)
	older := active + ".2"
	previous := active + ".1"
	_ = os.Remove(older)
	if err := os.Rename(previous, older); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(active, previous); err != nil && !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(active, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	sink.file = file
	sink.segmentBytes = 0
	sink.segmentCount = 0
	return nil
}

func devEventSegmentStats(file *os.File) (int64, int, error) {
	info, err := file.Stat()
	if err != nil {
		return 0, 0, err
	}
	reader, err := os.Open(file.Name())
	if err != nil {
		return 0, 0, err
	}
	defer reader.Close()
	count := 0
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxDevEventBytes*2)
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	return info.Size(), count, nil
}

func readDevEventSegments(paths []string, cursor int64, limit int, observedNext int64) ([]DevEvent, int64, bool, int64, error) {
	events := make([]DevEvent, 0, min(limit+1, maxDevEvents+1))
	earliest := observedNext
	truncated := false
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, observedNext, false, earliest, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), maxDevEventBytes*2)
		for scanner.Scan() {
			var event DevEvent
			if err := jsonapi.Unmarshal(scanner.Bytes(), &event); err != nil {
				continue
			}
			if event.Cursor < earliest {
				earliest = event.Cursor
			}
			observedNext = max(observedNext, event.Cursor+1)
			if event.Cursor < cursor {
				continue
			}
			events = append(events, event)
			if len(events) > limit {
				events = events[len(events)-limit:]
				truncated = true
			}
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		if scanErr != nil {
			return nil, observedNext, false, earliest, scanErr
		}
		if closeErr != nil {
			return nil, observedNext, false, earliest, closeErr
		}
	}
	return events, observedNext, truncated, earliest, nil
}

func devEventSegmentPaths(active string) []string {
	paths := make([]string, 0, 3)
	for _, path := range []string{active + ".2", active + ".1", active} {
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		} else if !os.IsNotExist(err) && !errors.Is(err, os.ErrNotExist) {
			continue
		}
	}
	return paths
}
