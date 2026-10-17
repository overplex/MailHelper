// Package i18n holds the user-visible text in the two languages the
// application offers. Every string the UI or the generated content.html can
// show lives in the Strings struct, so adding a language is one literal.
package i18n

// Lang identifies a translation. English is the default, which is why it is
// the zero value.
type Lang int

const (
	EN Lang = iota
	RU
)

// Count is the number of supported languages; the combo box is filled from
// Names in this order.
const Count = 2

// Names are the language names as shown in the language combo box. They are
// deliberately not translated: each is written in its own language.
var Names = [Count]string{"English", "Русский"}

// codes are the identifiers a language is stored under in the settings.
var codes = [Count]string{"en", "ru"}

// Code returns the identifier the language is stored under.
func (l Lang) Code() string {
	if l < 0 || int(l) >= Count {
		return codes[EN]
	}
	return codes[l]
}

// FromCode is the reverse of Code. ok is false for an unknown identifier.
func FromCode(code string) (l Lang, ok bool) {
	for i, c := range codes {
		if c == code {
			return Lang(i), true
		}
	}
	return EN, false
}

// langRussian is LANG_RUSSIAN, the primary language identifier Windows uses
// for every Russian locale.
const langRussian = 0x19

// FromWindowsLangID picks the language to start in for a Windows LANGID.
// Only the primary language - the low ten bits - is looked at, so ru-RU and
// ru-MD both count as Russian. Anything this application does not translate
// falls back to English, as does a zero LANGID, which is what the system
// reports when it cannot say.
func FromWindowsLangID(langID uint16) Lang {
	if langID&0x03FF == langRussian {
		return RU
	}
	return EN
}

// Strings is the full set of user-visible text for one language.
type Strings struct {
	AppTitle string

	Language   string
	Theme      string
	ThemeLight string
	ThemeDark  string

	GroupMailbox string
	Email        string
	AppPassword  string
	ImapServer   string
	ImapPort     string
	Username     string
	TestConn     string
	RefreshList  string

	GroupCopy     string
	GroupMove     string
	GroupDelete   string
	SourceFolder  string
	TargetFolder  string
	FolderInbox   string
	MoveAfterCopy string
	ChooseSaveDir string
	NoSaveDir     string
	DateFrom      string
	DateTo        string
	Start         string
	Stop          string
	BrowseTitle   string

	CapInfo    string
	CapError   string
	CapWarning string
	CapConfirm string

	ConnOK   string
	ConnFail string

	FoldersUpdated string

	ErrNoEmail    string
	ErrNoPassword string
	ErrNoServer   string
	ErrBadPort    string
	ErrNoSource   string
	ErrNoTarget   string
	ErrSameFolder string
	ErrNoSaveDir  string
	ErrDateOrder  string

	Stopped     string
	NothingToDo string
	CopyDone    string
	MoveDone    string
	DeleteDone  string
	Failed      string

	DeleteConfirm string

	ErrSaveSettings string

	// Labels used inside the generated content.html file.
	HTMLSender      string
	HTMLSenderEmail string
	HTMLReceived    string
	HTMLSubject     string
	HTMLAttachments string
	HTMLNoBody      string
}

var table = [Count]Strings{
	EN: {
		AppTitle: "MailHelper",

		Language:   "Language",
		Theme:      "Theme",
		ThemeLight: "Light",
		ThemeDark:  "Dark",

		GroupMailbox: "Mailbox",
		Email:        "E-mail",
		AppPassword:  "App password",
		ImapServer:   "IMAP server",
		ImapPort:     "IMAP port",
		Username:     "User name",
		TestConn:     "Test connection",
		RefreshList:  "Refresh folder list",

		GroupCopy:     "Copy to a folder on disk",
		GroupMove:     "Move from folder to folder on the server",
		GroupDelete:   "Delete messages",
		SourceFolder:  "Source folder on the server",
		TargetFolder:  "Target folder on the server",
		FolderInbox:   "Inbox",
		MoveAfterCopy: "Move on the server after copying to disk",
		ChooseSaveDir: "Choose the folder to save into",
		NoSaveDir:     "(no folder chosen)",
		DateFrom:      "From date",
		DateTo:        "To date",
		Start:         "Start",
		Stop:          "Stop",
		BrowseTitle:   "Select the folder messages will be saved into",

		CapInfo:    "MailHelper",
		CapError:   "Error",
		CapWarning: "Check the settings",
		CapConfirm: "Confirmation",

		ConnOK:   "Connected successfully.\n\nServer: %s\nUser: %s",
		ConnFail: "Could not connect.\n\n%s",

		FoldersUpdated: "The folder list has been refreshed.\n\nFolders found: %d",

		ErrNoEmail:    "Enter the e-mail address.",
		ErrNoPassword: "Enter the app password.",
		ErrNoServer:   "Enter the IMAP server.",
		ErrBadPort:    "The IMAP port must be a number between 1 and 65535.",
		ErrNoSource:   "Choose the source folder on the server.",
		ErrNoTarget:   "Choose the target folder on the server.",
		ErrSameFolder: "The source and target folders must be different.",
		ErrNoSaveDir:  "Choose the folder to save into.",
		ErrDateOrder:  "\"From date\" must not be later than \"To date\".",

		Stopped:     "Stopped by the user.\n\nMessages processed: %d of %d",
		NothingToDo: "No messages match the selected period.",
		CopyDone:    "Copying finished.\n\nMessages saved: %d\nFolder: %s",
		MoveDone:    "Moving finished.\n\nMessages moved: %d\nTarget folder: %s",
		DeleteDone:  "Deleting finished.\n\nMessages deleted: %d\nFolder: %s",
		Failed:      "The operation stopped with an error.\n\n%s\n\nMessages processed: %d of %d",

		DeleteConfirm: "Are you sure? The messages will be deleted from your mailbox.\n\nFolder: %s",

		ErrSaveSettings: "Could not save the settings.\n\n%s",

		HTMLSender:      "Sender name",
		HTMLSenderEmail: "Sender e-mail",
		HTMLReceived:    "Date and time received",
		HTMLSubject:     "Subject",
		HTMLAttachments: "Attachments",
		HTMLNoBody:      "(the message has no text body)",
	},
	RU: {
		AppTitle: "MailHelper",

		Language:   "Язык",
		Theme:      "Тема",
		ThemeLight: "Светлая",
		ThemeDark:  "Тёмная",

		GroupMailbox: "Почтовый ящик",
		Email:        "E-mail",
		AppPassword:  "Пароль для приложения",
		ImapServer:   "Сервер IMAP",
		ImapPort:     "Порт IMAP",
		Username:     "Имя пользователя",
		TestConn:     "Тест подключения",
		RefreshList:  "Обновить список папок",

		GroupCopy:     "Копировать в папку на диск",
		GroupMove:     "Перенести из папки в папку на почте",
		GroupDelete:   "Удалить письма",
		SourceFolder:  "Исходная папка на почте",
		TargetFolder:  "Целевая папка на почте",
		FolderInbox:   "Входящие",
		MoveAfterCopy: "Перенести из папки в папку на почте после копирования на диск",
		ChooseSaveDir: "Выбрать папку для сохранения",
		NoSaveDir:     "(папка не выбрана)",
		DateFrom:      "С даты",
		DateTo:        "До даты",
		Start:         "Запустить",
		Stop:          "Остановить",
		BrowseTitle:   "Выберите папку, в которую будут сохранены письма",

		CapInfo:    "MailHelper",
		CapError:   "Ошибка",
		CapWarning: "Проверьте настройки",
		CapConfirm: "Подтверждение",

		ConnOK:   "Подключение выполнено успешно.\n\nСервер: %s\nПользователь: %s",
		ConnFail: "Не удалось подключиться.\n\n%s",

		FoldersUpdated: "Список папок обновлён.\n\nНайдено папок: %d",

		ErrNoEmail:    "Укажите адрес e-mail.",
		ErrNoPassword: "Укажите пароль для приложения.",
		ErrNoServer:   "Укажите сервер IMAP.",
		ErrBadPort:    "Порт IMAP должен быть числом от 1 до 65535.",
		ErrNoSource:   "Выберите исходную папку на почте.",
		ErrNoTarget:   "Выберите целевую папку на почте.",
		ErrSameFolder: "Исходная и целевая папки должны различаться.",
		ErrNoSaveDir:  "Выберите папку для сохранения.",
		ErrDateOrder:  "Дата в поле «С даты» не должна быть позже даты в поле «До даты».",

		Stopped:     "Остановлено пользователем.\n\nОбработано писем: %d из %d",
		NothingToDo: "За выбранный период писем не найдено.",
		CopyDone:    "Копирование завершено.\n\nСохранено писем: %d\nПапка: %s",
		MoveDone:    "Перенос завершён.\n\nПеренесено писем: %d\nЦелевая папка: %s",
		DeleteDone:  "Удаление завершено.\n\nУдалено писем: %d\nПапка: %s",
		Failed:      "Операция остановлена из-за ошибки.\n\n%s\n\nОбработано писем: %d из %d",

		DeleteConfirm: "Вы уверены? Письма будут удалены в вашем почтовом ящике.\n\nПапка: %s",

		ErrSaveSettings: "Не удалось сохранить настройки.\n\n%s",

		HTMLSender:      "Имя отправителя",
		HTMLSenderEmail: "E-mail отправителя",
		HTMLReceived:    "Дата и время получения",
		HTMLSubject:     "Тема",
		HTMLAttachments: "Вложения",
		HTMLNoBody:      "(в письме нет текстовой части)",
	},
}

// Get returns the strings for a language, falling back to English for an
// out-of-range value.
func Get(l Lang) *Strings {
	if l < 0 || int(l) >= Count {
		l = EN
	}
	return &table[l]
}
