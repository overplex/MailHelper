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

// batchSize is how many messages one iteration handles. For the copy job
// the batch bounds the work between progress updates and the optional move
// that follows it; for the move and delete jobs it is the size of one round
// of server commands.
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

// job is everything a worker needs, captured on the UI thread before the
// goroutine starts so that it never touches a control.
type job struct {
	task     task
	cfg      imapc.Config
	source   imapc.Folder
	target   imapc.Folder
	from, to *time.Time

	// Copy only.
	root      string
	moveAfter bool
	tr        *i18n.Strings
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
			a.saveSettings()
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
	case taskDelete:
		return a.pbDelete
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

	case e.task == taskDelete:
		a.info(fmt.Sprintf(tr.DeleteDone, e.done, e.target))
	}
}

// ------------------------------------------------------------- Task control

func (a *App) beginTask(t task) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	a.running = t
	a.cancel = cancel
	a.updateEnabled()
	a.updateRunButtons()
	if bar := a.barFor(t); bar != 0 {
		a.setProgress(bar, 0, 0)
	}
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

// stopRunning asks the worker to stop at the next message or batch boundary.
func (a *App) stopRunning() {
	if a.cancel != nil {
		a.cancel()
	}
}

// updateEnabled switches the input controls off while a job runs, leaving
// only the Stop button of that job usable. When idle, a date picker is
// usable only while its filter is switched on.
func (a *App) updateEnabled() {
	idle := a.running == taskNone
	for _, h := range a.interactive() {
		winapi.EnableWindow(h, idle)
	}
	if idle {
		for _, d := range a.dateFilters() {
			winapi.EnableWindow(d.picker, winapi.Checked(d.check))
		}
		return
	}
	switch a.running {
	case taskCopy:
		winapi.EnableWindow(a.btnCopyRun, true)
	case taskMove:
		winapi.EnableWindow(a.btnMoveRun, true)
	case taskDelete:
		winapi.EnableWindow(a.btnDeleteRun, true)
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

// prepare gathers what every server job needs: the mailbox settings, the
// source folder and the period. ok is false when the user has been told
// what is missing.
func (a *App) prepare(t task, sourceCombo uintptr, noSource string, fromFilter, toFilter dateFilter) (j job, ok bool) {
	cfg, problem := a.mailboxConfig()
	if problem != "" {
		a.warn(problem)
		return job{}, false
	}
	source, found := a.selectedFolder(sourceCombo)
	if !found {
		a.warn(noSource)
		return job{}, false
	}
	from, to, problem := a.period(fromFilter, toFilter)
	if problem != "" {
		a.warn(problem)
		return job{}, false
	}
	return job{task: t, cfg: cfg, source: source, from: from, to: to}, true
}

// target reads a target folder combo and checks it differs from the source.
func (a *App) target(j *job, combo uintptr) bool {
	target, ok := a.selectedFolder(combo)
	if !ok {
		a.warn(a.tr.ErrNoTarget)
		return false
	}
	if target.Raw == j.source.Raw {
		a.warn(a.tr.ErrSameFolder)
		return false
	}
	j.target = target
	return true
}

func (a *App) startCopy() {
	j, ok := a.prepare(taskCopy, a.cbCopySrc, a.tr.ErrNoSource, a.copyFrom, a.copyTo)
	if !ok {
		return
	}
	j.moveAfter = a.moveAfterCopy()
	if j.moveAfter && !a.target(&j, a.cbCopyDst) {
		return
	}
	if a.saveDir == "" {
		a.warn(a.tr.ErrNoSaveDir)
		return
	}
	j.root, j.tr = a.saveDir, a.tr

	ctx := a.beginTask(taskCopy)
	go a.copyJob(ctx, j)
}

func (a *App) startMove() {
	j, ok := a.prepare(taskMove, a.cbMoveSrc, a.tr.ErrNoSource, a.moveFrom, a.moveTo)
	if !ok || !a.target(&j, a.cbMoveDst) {
		return
	}

	ctx := a.beginTask(taskMove)
	go a.serverJob(ctx, j, j.target.Name, func(s *imapc.Session, batch []uint32) error {
		return s.Move(batch, j.target.Raw)
	})
}

func (a *App) startDelete() {
	j, ok := a.prepare(taskDelete, a.cbDeleteSrc, a.tr.ErrNoTarget, a.deleteFrom, a.deleteTo)
	if !ok {
		return
	}
	label := j.source.DisplayName(a.tr.FolderInbox)
	if !a.confirm(fmt.Sprintf(a.tr.DeleteConfirm, label)) {
		return
	}

	ctx := a.beginTask(taskDelete)
	go a.serverJob(ctx, j, label, (*imapc.Session).Delete)
}

// openAndSearch connects, opens the source folder and lists the messages of
// the period.
func openAndSearch(j job) (*imapc.Session, []uint32, error) {
	session, err := imapc.Open(j.cfg)
	if err != nil {
		return nil, nil, err
	}
	if _, err := session.Select(j.source.Raw); err != nil {
		session.Close()
		return nil, nil, err
	}
	uids, err := session.SearchUIDs(j.from, j.to)
	if err != nil {
		session.Close()
		return nil, nil, err
	}
	return session, uids, nil
}

// copyJob saves every message of the period into the chosen folder on disk,
// ten at a time, and - when the box is ticked - moves each batch on the
// server once its messages are safely written. Whether the source is a
// folder of outgoing mail decides whether the message folders are named
// after the sender or after the recipient.
func (a *App) copyJob(ctx context.Context, j job) {
	done, total := 0, 0
	finish := func(err error, stopped bool) {
		a.post(event{
			task: taskCopy, kind: evDone,
			done: done, total: total, err: err, stopped: stopped,
			target: j.root,
		})
	}

	session, uids, err := openAndSearch(j)
	if err != nil {
		finish(err, false)
		return
	}
	defer session.Close()

	total = len(uids)
	a.post(event{task: taskCopy, kind: evProgress, done: 0, total: total})

	fromSent := j.source.IsSent()
	for start := 0; start < len(uids); start += batchSize {
		batch := uids[start:min(start+batchSize, len(uids))]

		saved := 0
		for _, uid := range batch {
			if ctx.Err() != nil {
				break
			}
			message, err := session.Fetch(uid)
			if err != nil {
				finish(err, false)
				return
			}
			if _, err := mailsave.Save(j.root, message.Raw, message.InternalDate, j.tr, fromSent); err != nil {
				finish(err, false)
				return
			}
			saved++
			done++
			a.post(event{task: taskCopy, kind: evProgress, done: done, total: total})
		}

		// Only messages already on disk are moved - including those of a
		// batch cut short by Stop - so an interrupted run never loses one
		// and never leaves one behind to be copied a second time.
		if j.moveAfter && saved > 0 {
			if err := session.Move(batch[:saved], j.target.Raw); err != nil {
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

// serverJob applies op to the messages of the period, ten per iteration:
// the move job's COPY, \Deleted and EXPUNGE, or the delete job's \Deleted
// and EXPUNGE. Any error from the server stops the run immediately. label
// names the folder for the closing report.
func (a *App) serverJob(ctx context.Context, j job, label string, op func(*imapc.Session, []uint32) error) {
	done, total := 0, 0
	finish := func(err error, stopped bool) {
		a.post(event{
			task: j.task, kind: evDone,
			done: done, total: total, err: err, stopped: stopped,
			target: label,
		})
	}

	session, uids, err := openAndSearch(j)
	if err != nil {
		finish(err, false)
		return
	}
	defer session.Close()

	total = len(uids)
	a.post(event{task: j.task, kind: evProgress, done: 0, total: total})

	for start := 0; start < len(uids); start += batchSize {
		if ctx.Err() != nil {
			finish(nil, true)
			return
		}
		batch := uids[start:min(start+batchSize, len(uids))]
		if err := op(session, batch); err != nil {
			finish(err, false)
			return
		}
		done += len(batch)
		a.post(event{task: j.task, kind: evProgress, done: done, total: total})
	}

	finish(nil, false)
}
