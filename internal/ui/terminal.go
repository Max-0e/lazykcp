package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

const terminalPaneBorderSize = 2

type terminalSession struct {
	id             uint64
	workspace      string
	command        *exec.Cmd
	pty            *os.File
	emulator       *vt.Emulator
	kubeconfigPath string
	cleanupConfig  func() error
	readerDone     bool
	processExited  bool
	processErr     error
	readerErr      error
	failure        string
	waitOnce       sync.Once
	waitDone       chan struct{}
	waitErr        error
}

type terminalOutputMsg struct {
	id   uint64
	data []byte
	err  error
}

type terminalExitMsg struct {
	id  uint64
	err error
}

type terminalInputDoneMsg struct {
	id  uint64
	err error
}

func (m *Model) startTerminal() tea.Cmd {
	if m.terminal != nil {
		return nil
	}
	node := m.findWorkspace(m.workspaceCursor)
	if node == nil {
		m.status = "Cannot start terminal: no workspace is selected"
		return nil
	}

	kubeconfigPath, cleanupConfig, err := m.client.CreateWorkspaceKubeconfig(node.path)
	if err != nil {
		m.status = "Cannot start terminal: " + err.Error()
		return nil
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	command := exec.Command(shell)
	command.Env = terminalEnvironment(os.Environ(), kubeconfigPath)
	size := m.terminalPTYSize()
	ptyFile, err := pty.StartWithSize(command, size)
	if err != nil {
		if cleanupErr := cleanupConfig(); cleanupErr != nil {
			m.status = fmt.Sprintf("Cannot start terminal: %v (temporary kubeconfig cleanup: %v)", err, cleanupErr)
		} else {
			m.status = "Cannot start terminal: " + err.Error()
		}
		return nil
	}

	m.terminalSequence++
	session := &terminalSession{
		id:             m.terminalSequence,
		workspace:      node.path,
		command:        command,
		pty:            ptyFile,
		emulator:       vt.NewEmulator(int(size.Cols), int(size.Rows)),
		kubeconfigPath: kubeconfigPath,
		cleanupConfig:  cleanupConfig,
		waitDone:       make(chan struct{}),
	}
	m.terminal = session
	m.terminalReturnFocus = m.focus
	m.focus = detailFocus
	m.status = ""
	return tea.Batch(readTerminalOutput(session), waitTerminalProcess(session), forwardTerminalInput(session))
}

func terminalEnvironment(environment []string, kubeconfigPath string) []string {
	updated := make([]string, 0, len(environment)+2)
	for _, entry := range environment {
		if strings.HasPrefix(entry, "KUBECONFIG=") || strings.HasPrefix(entry, "TERM=") {
			continue
		}
		updated = append(updated, entry)
	}
	return append(updated, "KUBECONFIG="+kubeconfigPath, "TERM=xterm-256color")
}

func terminalPTYSize(width, height int) *pty.Winsize {
	maxDimension := int(^uint16(0))
	return &pty.Winsize{
		Cols: uint16(clamp(width-terminalPaneBorderSize, 1, maxDimension)),
		Rows: uint16(clamp(height-terminalPaneBorderSize, 1, maxDimension)),
	}
}

func (m *Model) terminalPTYSize() *pty.Winsize {
	bounds := m.detailPaneBounds()
	return terminalPTYSize(bounds.width, bounds.height)
}

func (m *Model) terminalLines() []string {
	if m.terminal == nil {
		return nil
	}
	rendered := strings.TrimSuffix(m.terminal.emulator.Render(), "\n")
	if rendered == "" {
		return nil
	}
	return strings.Split(rendered, "\n")
}

func readTerminalOutput(session *terminalSession) tea.Cmd {
	return func() tea.Msg {
		buffer := make([]byte, 4096)
		n, err := session.pty.Read(buffer)
		return terminalOutputMsg{
			id:   session.id,
			data: append([]byte(nil), buffer[:n]...),
			err:  err,
		}
	}
}

func waitTerminalProcess(session *terminalSession) tea.Cmd {
	return func() tea.Msg {
		return terminalExitMsg{id: session.id, err: session.wait()}
	}
}

func (s *terminalSession) wait() error {
	s.waitOnce.Do(func() {
		s.waitErr = s.command.Wait()
		close(s.waitDone)
	})
	<-s.waitDone
	return s.waitErr
}

func forwardTerminalInput(session *terminalSession) tea.Cmd {
	return func() tea.Msg {
		_, err := io.Copy(session.pty, session.emulator)
		return terminalInputDoneMsg{id: session.id, err: err}
	}
}

func (m *Model) handleTerminalOutput(message terminalOutputMsg) tea.Cmd {
	session := m.terminal
	if session == nil || session.id != message.id {
		return nil
	}
	if len(message.data) > 0 {
		if _, err := session.emulator.Write(message.data); err != nil {
			session.readerErr = fmt.Errorf("parse terminal output: %w", err)
			session.failure = session.readerErr.Error()
			if killErr := session.command.Process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
				session.failure += "; stop shell: " + killErr.Error()
			}
			session.readerDone = true
			return m.finishTerminalIfReady(session)
		}
	}
	if message.err != nil {
		session.readerDone = true
		if !terminalReadClosed(message.err) {
			session.readerErr = fmt.Errorf("read terminal output: %w", message.err)
			session.failure = session.readerErr.Error()
			if err := session.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				session.failure += "; stop shell: " + err.Error()
			}
		}
		return m.finishTerminalIfReady(session)
	}
	return readTerminalOutput(session)
}

func terminalReadClosed(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, os.ErrClosed) ||
		errors.Is(err, syscall.EIO)
}

func (m *Model) handleTerminalExit(message terminalExitMsg) tea.Cmd {
	session := m.terminal
	if session == nil || session.id != message.id {
		return nil
	}
	session.processExited = true
	session.processErr = message.err
	return m.finishTerminalIfReady(session)
}

func (m *Model) handleTerminalInputDone(message terminalInputDoneMsg) tea.Cmd {
	session := m.terminal
	if session == nil || session.id != message.id || session.processExited {
		return nil
	}
	if message.err != nil && !errors.Is(message.err, io.EOF) && !errors.Is(message.err, os.ErrClosed) {
		session.failure = "terminal input forwarding failed: " + message.err.Error()
		if err := session.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			session.failure += "; stop shell: " + err.Error()
		}
	}
	return nil
}

func (m *Model) finishTerminalIfReady(session *terminalSession) tea.Cmd {
	if !session.processExited || !session.readerDone || m.terminal != session {
		return nil
	}
	m.terminal = nil
	m.focus = m.terminalReturnFocus
	if err := session.close(); err != nil {
		m.status = err.Error()
	} else if session.failure != "" {
		m.status = session.failure
	} else if session.processErr != nil && session.command.ProcessState != nil {
		m.status = fmt.Sprintf("Terminal exited with status %d", session.command.ProcessState.ExitCode())
	}
	return nil
}

func (s *terminalSession) close() error {
	var closeErrors []error
	if err := s.emulator.Close(); err != nil {
		closeErrors = append(closeErrors, fmt.Errorf("close terminal emulator: %w", err))
	}
	if err := s.pty.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		closeErrors = append(closeErrors, fmt.Errorf("close terminal PTY: %w", err))
	}
	if err := s.cleanupConfig(); err != nil {
		closeErrors = append(closeErrors, err)
	}
	return errors.Join(closeErrors...)
}

func (m *Model) resizeTerminal() {
	session := m.terminal
	if session == nil {
		return
	}
	size := m.terminalPTYSize()
	if err := pty.Setsize(session.pty, size); err != nil {
		session.failure = "resize terminal: " + err.Error()
		return
	}
	session.emulator.Resize(int(size.Cols), int(size.Rows))
	session.failure = ""
}

func (m *Model) handleTerminalKey(key tea.KeyMsg) {
	if m.terminal == nil {
		return
	}
	if key.Paste {
		m.terminal.emulator.Paste(string(key.Runes))
		return
	}
	event, ok := terminalKeyEvent(key)
	if ok {
		m.terminal.emulator.SendKey(event)
	}
}

func terminalKeyEvent(key tea.KeyMsg) (vt.KeyPressEvent, bool) {
	if key.Type == tea.KeyRunes {
		if len(key.Runes) == 0 {
			return vt.KeyPressEvent{}, false
		}
		mod := vt.KeyMod(0)
		if key.Alt {
			mod |= vt.ModAlt
		}
		return vt.KeyPressEvent{Code: key.Runes[0], Text: string(key.Runes), Mod: mod}, true
	}

	name := key.String()
	mod := vt.KeyMod(0)
	for {
		switch {
		case strings.HasPrefix(name, "alt+"):
			mod |= vt.ModAlt
			name = strings.TrimPrefix(name, "alt+")
		case strings.HasPrefix(name, "ctrl+"):
			mod |= vt.ModCtrl
			name = strings.TrimPrefix(name, "ctrl+")
		case strings.HasPrefix(name, "shift+"):
			mod |= vt.ModShift
			name = strings.TrimPrefix(name, "shift+")
		default:
			goto resolve
		}
	}

resolve:
	if mod&vt.ModCtrl != 0 && len([]rune(name)) == 1 {
		return vt.KeyPressEvent{Code: []rune(strings.ToLower(name))[0], Mod: mod}, true
	}
	keyCodes := map[string]rune{
		"enter": vt.KeyEnter, "backspace": vt.KeyBackspace, "tab": vt.KeyTab,
		"esc": vt.KeyEscape, "escape": vt.KeyEscape, "space": vt.KeySpace, " ": vt.KeySpace,
		"up": vt.KeyUp, "down": vt.KeyDown, "left": vt.KeyLeft, "right": vt.KeyRight,
		"home": vt.KeyHome, "end": vt.KeyEnd, "pgup": vt.KeyPgUp, "pageup": vt.KeyPgUp,
		"pgdown": vt.KeyPgDown, "pagedown": vt.KeyPgDown, "insert": vt.KeyInsert, "delete": vt.KeyDelete,
		"f1": vt.KeyF1, "f2": vt.KeyF2, "f3": vt.KeyF3, "f4": vt.KeyF4,
		"f5": vt.KeyF5, "f6": vt.KeyF6, "f7": vt.KeyF7, "f8": vt.KeyF8,
		"f9": vt.KeyF9, "f10": vt.KeyF10, "f11": vt.KeyF11, "f12": vt.KeyF12,
	}
	code, ok := keyCodes[name]
	if !ok {
		return vt.KeyPressEvent{}, false
	}
	if mod&vt.ModCtrl != 0 && code == vt.KeySpace {
		mod |= vt.ModCtrl
	}
	return vt.KeyPressEvent{Code: code, Mod: mod}, true
}

func (m *Model) handleTerminalMouse(message tea.MouseMsg) {
	session := m.terminal
	bounds := m.detailPaneBounds()
	message, ok := terminalPaneMouseMessage(bounds, message)
	if session == nil || !ok {
		return
	}
	event, ok := terminalMouseEvent(message)
	if !ok {
		return
	}
	session.emulator.SendMouse(event)
}

func terminalPaneMouseMessage(bounds paneBounds, message tea.MouseMsg) (tea.MouseMsg, bool) {
	if message.X <= bounds.x || message.X >= bounds.x+bounds.width-1 ||
		message.Y <= bounds.y || message.Y >= bounds.y+bounds.height-1 {
		return tea.MouseMsg{}, false
	}
	message.X -= bounds.x + 1
	message.Y -= bounds.y + 1
	return message, true
}

func terminalMouseEvent(message tea.MouseMsg) (uv.MouseEvent, bool) {
	mouse := uv.Mouse{X: message.X, Y: message.Y}
	if message.Shift {
		mouse.Mod |= vt.ModShift
	}
	if message.Alt {
		mouse.Mod |= vt.ModAlt
	}
	if message.Ctrl {
		mouse.Mod |= vt.ModCtrl
	}
	switch message.Button {
	case tea.MouseButtonLeft:
		mouse.Button = vt.MouseLeft
	case tea.MouseButtonMiddle:
		mouse.Button = vt.MouseMiddle
	case tea.MouseButtonRight:
		mouse.Button = vt.MouseRight
	case tea.MouseButtonWheelUp:
		mouse.Button = uv.MouseWheelUp
		return uv.MouseWheelEvent(mouse), true
	case tea.MouseButtonWheelDown:
		mouse.Button = uv.MouseWheelDown
		return uv.MouseWheelEvent(mouse), true
	default:
		mouse.Button = uv.MouseNone
	}
	switch message.Action {
	case tea.MouseActionPress:
		return uv.MouseClickEvent(mouse), true
	case tea.MouseActionRelease:
		return uv.MouseReleaseEvent(mouse), true
	case tea.MouseActionMotion:
		return uv.MouseMotionEvent(mouse), true
	default:
		return nil, false
	}
}

func (m *Model) Close() error {
	session := m.terminal
	if session == nil {
		return nil
	}
	m.terminal = nil
	var closeErrors []error
	if session.command.Process != nil && !session.processExited {
		if err := session.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			closeErrors = append(closeErrors, fmt.Errorf("stop terminal shell: %w", err))
		}
	}
	session.wait()
	m.focus = m.terminalReturnFocus
	if err := session.close(); err != nil {
		closeErrors = append(closeErrors, err)
	}
	return errors.Join(closeErrors...)
}
