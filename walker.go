package powerwalk

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// DefaultConcurrentWalks is the default number of files that will be walked at the
// same time.
const DefaultConcurrentWalks int = 50

// Walk walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root. All errors that arise visiting files
// and directories are filtered by walkFn. The output is non-deterministic.
// WalkLimit does not follow symbolic links.
//
// For each file and directory encountered, Walk will trigger a new Go routine
// allowing you to handle each item concurrently.  A maximum of DefaultConcurrentWalks
// walkFns will be called at any one time.
func Walk(root string, walkFn filepath.WalkFunc) error {
	return WalkLimit(root, walkFn, DefaultConcurrentWalks)
}

// WalkLimit walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root. All errors that arise visiting files
// and directories are filtered by walkFn. The output is non-deterministic.
// WalkLimit does not follow symbolic links.
//
// For each file and directory encountered, Walk will trigger a new Go routine
// allowing you to handle each item concurrently.  A maximum of limit walkFns will
// be called at any one time.
func WalkLimit(root string, walkFn filepath.WalkFunc, limit int) error {

	// make sure limit is sensible
	if limit < 1 {
		panic("powerwalk: limit must be greater than zero.")
	}

	files := make(chan *walkArgs)
	kill := make(chan struct{})
	errs := make(chan error)

	for i := 0; i < limit; i++ {
		go func(i int) {
			for {
				select {
				case file := <-files:
					if err := walkFn(file.path, file.info, file.err); err != nil {
						errs <- err
					}
				case <-kill:
					return
				}
			}
		}(i)
	}

	var walkErr error

	// check for errors
	go func() {
		select {
		case walkErr = <-errs:
			close(kill)
		case <-kill:
			return
		}
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {

		filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			select {
			case <-kill:
				close(files)
				return errors.New("Error in walk. Cannot continue.")
			default:
				select {
				case files <- &walkArgs{path: p, info: info, err: err}:
				default:
				}
				return nil
			}
		})

		wg.Done()
	}()

	wg.Wait()

	if walkErr == nil {
		close(kill)
	}

	return walkErr
}

type walkArgs struct {
	path string
	info os.FileInfo
	err  error
}
