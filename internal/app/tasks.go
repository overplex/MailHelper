//go:build windows

package app

import (
	"context"
	"fmt"
	"time"

	"mailhelper/internal/i18n"
	"mailhelper/internal/imapc"
	"mailhelper/internal/mailsave"
	"mailhelper/internal/winapi"
)

// batchSize is how many messages one iteration handles. For the copy job the batch bounds the work between progress
// updates and the optional move that follows it; for the move job it is the
// size of one COPY / \Deleted / EXPUNGE round.
const batchSize = 10

type eventKind int

const (
	evProgress eventKind = iota
	evDone
)

// event is one message from the worker goroutine to the UI thread.
type event struct {
	task    task
	kind    eventKind
	done    int
	total   int
	stopped bool
	err     error
	folders []imapc.Folder
	server  string
	user    string
	target  string
}

// post hands an event to the UI thread and wakes it up. The channel carries
// the value; the window message only says "there is something to read".
func (a *App) post(e event) {
	a.events <- e
	winapi.PostMessage(a.hwnd, wmWorker, 0, 0)
}

func (a *App) drainEvents() {
	for {
		select {
		case e := <-a.events:
			a.handleEvent(e)
		default:
			return
		}
	}
}

func (a *App) handleEvent(e event) {
	switch e.kind {
	case evProgress:
		a.setProgress(a.barFor(e.task), e.done, e.total)

	case evDone:
		a.endTask()
		a.report(e)
		// The bar goes back to empty once the message box that announced
		// the result has been dismissed.
		a.setProgress(a.barFor(e.task), 0, 0)

		if a.closeRequested {
			winapi.DestroyWindow(a.hwnd)
		}
	}
}

func (a *App) barFor(t task) uintptr {
	switch t {
	case taskCopy:
		return a.pbCopy
	case taskMove:
		return a.pbMove
	}
	return 0
}

// report shows the pop-up that closes out a job.
func (a *App) report(e event) {
	tr := a.tr
	switch {
	case e.task == taskTest:
		if e.err != nil {
			a.fail(fmt.Sprintf(tr.ConnFail, e.err))
			return
		}
		a.info(fmt.Sprintf(tr.ConnOK, e.server, e.user))

	case e.task == taskFolders:
		if e.err != nil {
			a.fail(fmt.Sprintf(tr.ConnFail, e.err))
			return
		}
		a.fillFolders(e.folders)
		a.info(fmt.Sprintf(tr.FoldersUpdated, len(a.folders)))

	case e.err != nil:
		a.fail(fmt.Sprintf(tr.Failed, e.err, e.done, e.total))

	case e.stopped:
		a.info(fmt.Sprintf(tr.Stopped, e.done, e.total))

	case e.total == 0:
		a.info(tr.NothingToDo)

	case e.task == taskCopy:
		a.info(fmt.Sprintf(tr.CopyDone, e.done, e.target))

	case e.task == taskMove:
		a.info(fmt.Sprintf(tr.MoveDone, e.done, e.target))
	}
}

// ------------------------------------------------------------- Task control

func (a *App) beginTask(t task) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	a.running = t
	a.cancel = cancel
	a.updateEnabled()
	a.updateRunButtons()
	return ctx
}

func (a *App) endTask() {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.running = taskNone
	a.updateEnabled()
	a.updateRunButtons()
}

// stopRunning asks the worker to stop at the next batch boundary.
func (a *App) stopRunning() {
	if a.cancel != nil {
		a.cancel()
	}
}

// updateEnabled switches the input controls off while a job runs, leaving
// only the Stop button of that job usable.
func (a *App) updateEnabled() {
	idle := a.running == taskNone
	for _, h := range a.interactive() {
		winapi.EnableWindow(h, idle)
	}
	if idle {
		return
	}
	switch a.running {
	case taskCopy:
		winapi.EnableWindow(a.btnCopyRun, true)
	case taskMove:
		winapi.EnableWindow(a.btnMoveRun, true)
	}
}

// ------------------------------------------------------------------- Jobs

func (a *App) startTest() {
	cfg, problem := a.mailboxConfig()
	if problem != "" {
		a.warn(problem)
		return
	}

	a.beginTask(taskTest)
	go func() {
		session, err := imapc.Open(cfg)
		if err == nil {
			session.Close()
		}
		a.post(event{
			task: taskTest, kind: evDone, err: err,
			server: cfg.Server, user: cfg.Username,
		})
	}()
}

func (a *App) startRefresh() {
	cfg, problem := a.mailboxConfig()
	if problem != "" {
		a.warn(problem)
		return
	}

	a.beginTask(taskFolders)
	go func() {
		session, err := imapc.Open(cfg)
		if err != nil {
			a.post(event{task: taskFolders, kind: evDone, err: err})
			return
		}
		defer session.Close()

		folders, err := session.Folders()
		a.post(event{task: taskFolders, kind: evDone, err: err, folders: folders})
	}()
}

func (a *App) startCopy() {
	cfg, problem := a.mailboxConfig()
	if problem != "" {
		a.warn(problem)
		return
	}

	source, ok := a.selectedFolder(a.cbCopySrc)
	if !ok {
		a.warn(a.tr.ErrNoSource)
		return
	}

	moveAfter := a.moveAfterCopy()
	var target imapc.Folder
	if moveAfter {
		target, ok = a.selectedFolder(a.cbCopyDst)
		if !ok {
			a.warn(a.tr.ErrNoTarget)
			return
		}
		if target.Raw == source.Raw {
			a.warn(a.tr.ErrSameFolder)
			return
		}
	}

	if a.saveDir == "" {
		a.warn(a.tr.ErrNoSaveDir)
		return
	}

	from, to := dateOf(a.dtCopyFrom), dateOf(a.dtCopyTo)
	if problem := a.checkDateOrder(from, to); problem != "" {
		a.warn(problem)
		return
	}

	root, tr := a.saveDir, a.tr
	ctx := a.beginTask(taskCopy)
	a.setProgress(a.pbCopy, 0, 0)

	go a.copyJob(ctx, cfg, source.Raw, target.Raw, moveAfter, root, from, to, tr, source.IsSent())
}

func (a *App) startMove() {
	cfg, problem := a.mailboxConfig()
	if problem != "" {
		a.warn(problem)
		return
	}

	source, ok := a.selectedFolder(a.cbMoveSrc)
	if !ok {
		a.warn(a.tr.ErrNoSource)
		return
	}
	target, ok := a.selectedFolder(a.cbMoveDst)
	if !ok {
		a.warn(a.tr.ErrNoTarget)
		return
	}
	if target.Raw == source.Raw {
		a.warn(a.tr.ErrSameFolder)
		return
	}

	from, to := dateOf(a.dtMoveFrom), dateOf(a.dtMoveTo)
	if problem := a.checkDateOrder(from, to); problem != "" {
		a.warn(problem)
		return
	}

	ctx := a.beginTask(taskMove)
	a.setProgress(a.pbMove, 0, 0)

	go a.moveJob(ctx, cfg, source.Raw, target.Raw, target.Name, from, to)
}

// copyJob saves every message of the period into the chosen folder on disk,
// ten at a time, and - when the box is ticked - moves each batch on the
// server once its messages are safely written. fromSent carries down whether
// the source is a folder of outgoing mail, which decides whether the message
// folders are named after the sender or after the recipient.
func (a *App) copyJob(ctx context.Context, cfg imapc.Config, sourceRaw, targetRaw string,
	moveAfter bool, root string, from, to *time.Time, tr *i18n.Strings, fromSent bool) {

	done, total := 0, 0
	finish := func(err error, stopped bool) {
		a.post(event{
			task: taskCopy, kind: evDone,
			done: done, total: total, err: err, stopped: stopped,
			target: root,
		})
	}

	session, err := imapc.Open(cfg)
	if err != nil {
		finish(err, false)
		return
	}
	defer session.Close()

	if _, err := session.Select(sourceRaw); err != nil {
		finish(err, false)
		return
	}
	uids, err := session.SearchUIDs(from, to)
	if err != nil {
		finish(err, false)
		return
	}

	total = len(uids)
	a.post(event{task: taskCopy, kind: evProgress, done: 0, total: total})

	for start := 0; start < len(uids); start += batchSize {
		end := start + batchSize
		if end > len(uids) {
			end = len(uids)
		}
		batch := uids[start:end]

		for _, uid := range batch {
			if ctx.Err() != nil {
				finish(nil, true)
				return
			}
			message, err := session.Fetch(uid)
			if err != nil {
				finish(err, false)
				return
			}
			if _, err := mailsave.Save(root, message.Raw, message.InternalDate, tr, fromSent); err != nil {
				finish(err, false)
				return
			}
			done++
			a.post(event{task: taskCopy, kind: evProgress, done: done, total: total})
		}

		if moveAfter {
			// Only messages already on disk are moved, so an interrupted
			// run never loses one.
			if err := session.Move(batch, targetRaw); err != nil {
				finish(err, false)
				return
			}
		}
		if ctx.Err() != nil {
			finish(nil, true)
			return
		}
	}

	finish(nil, false)
}

// moveJob moves the messages of the period from one server folder to
// another with COPY, the \Deleted flag and EXPUNGE, ten per iteration. Any
// error from the server stops the run immediately.
func (a *App) moveJob(ctx context.Context, cfg imapc.Config, sourceRaw, targetRaw, targetName string,
	from, to *time.Time) {

	done, total := 0, 0
	finish := func(err error, stopped bool) {
		a.post(event{
			task: taskMove, kind: evDone,
			done: done, total: total, err: err, stopped: stopped,
			target: targetName,
		})
	}

	session, err := imapc.Open(cfg)
	if err != nil {
		finish(err, false)
		return
	}
	defer session.Close()

	if _, err := session.Select(sourceRaw); err != nil {
		finish(err, false)
		return
	}
	uids, err := session.SearchUIDs(from, to)
	if err != nil {
		finish(err, false)
		return
	}

	total = len(uids)
	a.post(event{task: taskMove, kind: evProgress, done: 0, total: total})

	for start := 0; start < len(uids); start += batchSize {
		if ctx.Err() != nil {
			finish(nil, true)
			return
		}
		end := start + batchSize
		if end > len(uids) {
			end = len(uids)
		}
		batch := uids[start:end]

		if err := session.Move(batch, targetRaw); err != nil {
			finish(err, false)
			return
		}
		done += len(batch)
		a.post(event{task: taskMove, kind: evProgress, done: done, total: total})
	}

	finish(nil, false)
}
