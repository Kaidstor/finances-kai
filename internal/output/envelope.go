package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// SchemaVersion — версия конверта; растёт, когда меняется форма полей.
const SchemaVersion = 1

// Failure — ошибка в машинном виде. Kind — короткий класс (usage, config,
// auth, network, timeout, not_found, api, ambiguous, confirm, interrupted), по
// нему принимают решение, не читая текст.
type Failure struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// Envelope — то, что печатается в режиме --json.
//
// Exit дублирует код выхода: команду могут запустить фоном и читать результат
// из файла, где кода выхода уже нет.
type Envelope struct {
	V       int      `json:"v"`
	Command string   `json:"command"`
	Exit    int      `json:"exit"`
	Data    any      `json:"data"`
	Warning []string `json:"warning,omitempty"`
	Error   *Failure `json:"error"`
}

// Printer — режим вывода команды. Текст по умолчанию: команды печатают его
// сами. В режиме JSON всё, что уходит в stdout, — один конверт.
type Printer struct {
	JSON    bool
	Command string
	Out     io.Writer
	Err     io.Writer

	warnings []string
	emitted  bool
}

func New(asJSON bool) *Printer {
	return &Printer{JSON: asJSON, Out: os.Stdout, Err: os.Stderr}
}

// Info — куда писать служебный текст (вопрос подтверждения, приглашение ввести
// токен): в режиме JSON stdout занят конвертом.
func (p *Printer) Info() io.Writer {
	if p.JSON {
		return p.Err
	}
	return p.Out
}

// Warn копит предупреждение для поля warning; в текстовом режиме печатает его
// в stderr сразу. Код выхода оно не меняет.
func (p *Printer) Warn(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	if p.JSON {
		p.warnings = append(p.warnings, msg)
		return
	}
	fmt.Fprintln(p.Err, Yellow(msg))
}

// Data печатает успешный конверт. В текстовом режиме ничего не делает, так что
// вызывать её можно безусловно — но текст команда печатает сама.
func (p *Printer) Data(data any) error {
	if !p.JSON {
		return nil
	}
	p.encode(Envelope{V: SchemaVersion, Command: p.Command, Data: data, Warning: p.warnings})
	return nil
}

// Emitted — конверт уже напечатан.
func (p *Printer) Emitted() bool { return p.emitted }

// Fail печатает ошибку конвертом в stdout: разбирающая сторона не должна
// читать два потока, чтобы понять исход. Только для режима JSON — текстовый
// отказ печатает вызывающий.
func (p *Printer) Fail(code int, kind, message string) {
	p.encode(Envelope{
		V:       SchemaVersion,
		Command: p.Command,
		Exit:    code,
		Warning: p.warnings,
		Error:   &Failure{Kind: kind, Message: message},
	})
}

func (p *Printer) encode(e Envelope) {
	p.emitted = true
	enc := json.NewEncoder(p.Out)
	enc.SetIndent("", "  ")
	// Без этого & и < в описаниях платежей уезжают в & и <.
	enc.SetEscapeHTML(false)
	_ = enc.Encode(e)
}
