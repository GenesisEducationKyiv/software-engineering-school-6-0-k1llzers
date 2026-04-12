package mail

import (
	"bytes"
	"embed"
	"html/template"
)

const confirmationEmailSubject = "Підтвердження підписки на релізи GitHub"

//go:embed templates/*.tmpl
var templateFS embed.FS

type ConfirmationTemplateData struct {
	RepositoryFullName string
	ConfirmationURL    string
	CancellationURL    string
}

type ReleaseTemplateData struct {
	RepositoryFullName string
	TagName            string
	ReleaseURL         string
	CancellationURL    string
}

type RenderedEmail struct {
	Subject  string
	HTMLBody string
	TextBody string
}

type TemplateRenderer struct {
	confirmationEmail *template.Template
	releaseEmail      *template.Template
}

func NewTemplateRenderer() (*TemplateRenderer, error) {
	confirmationEmail, err := template.ParseFS(templateFS, "templates/confirmation_email.html.tmpl")
	if err != nil {
		return nil, err
	}

	releaseEmail, err := template.ParseFS(templateFS, "templates/release_email.html.tmpl")
	if err != nil {
		return nil, err
	}

	return &TemplateRenderer{
		confirmationEmail: confirmationEmail,
		releaseEmail:      releaseEmail,
	}, nil
}

func (r *TemplateRenderer) RenderConfirmationEmail(data ConfirmationTemplateData) (RenderedEmail, error) {
	payload := struct {
		Subject string
		ConfirmationTemplateData
	}{
		Subject:                  confirmationEmailSubject,
		ConfirmationTemplateData: data,
	}

	var htmlBody bytes.Buffer
	if err := r.confirmationEmail.Execute(&htmlBody, payload); err != nil {
		return RenderedEmail{}, err
	}

	return RenderedEmail{
		Subject:  confirmationEmailSubject,
		HTMLBody: htmlBody.String(),
		TextBody: "Для підтвердження підписки на нові релізи репозиторію " + data.RepositoryFullName + " відкрийте посилання: " + data.ConfirmationURL + "\nДля скасування підписки відкрийте посилання: " + data.CancellationURL,
	}, nil
}

func (r *TemplateRenderer) RenderReleaseEmail(data ReleaseTemplateData) (RenderedEmail, error) {
	subject := "Новий реліз " + data.RepositoryFullName + ": " + data.TagName
	payload := struct {
		Subject string
		ReleaseTemplateData
	}{
		Subject:             subject,
		ReleaseTemplateData: data,
	}

	var htmlBody bytes.Buffer
	if err := r.releaseEmail.Execute(&htmlBody, payload); err != nil {
		return RenderedEmail{}, err
	}

	return RenderedEmail{
		Subject:  subject,
		HTMLBody: htmlBody.String(),
		TextBody: "Для репозиторію " + data.RepositoryFullName + " вийшов новий реліз " + data.TagName + ": " + data.ReleaseURL + "\nДля скасування підписки відкрийте посилання: " + data.CancellationURL,
	}, nil
}
