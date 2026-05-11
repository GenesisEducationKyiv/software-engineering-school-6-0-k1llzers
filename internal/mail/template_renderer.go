package mail

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
)

const confirmationEmailSubject = "Confirm your GitHub release subscription"

const (
	templateKindConfirmation = "confirmation"
	templateKindRelease      = "release"
)

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
}

type renderFunc func(data any) (RenderedEmail, error)

type templateSpec struct {
	path   string
	render func(tmpl *template.Template, data any) (RenderedEmail, error)
}

type TemplateRenderer struct {
	renderers map[string]renderFunc
}

func NewTemplateRenderer() (*TemplateRenderer, error) {
	specs := map[string]templateSpec{
		templateKindConfirmation: newTemplateSpec(
			templateKindConfirmation,
			"templates/confirmation_email.html.tmpl",
			func(data ConfirmationTemplateData) (string, any) {
				return confirmationEmailSubject, struct {
					Subject string
					ConfirmationTemplateData
				}{
					Subject:                  confirmationEmailSubject,
					ConfirmationTemplateData: data,
				}
			},
		),
		templateKindRelease: newTemplateSpec(
			templateKindRelease,
			"templates/release_email.html.tmpl",
			func(data ReleaseTemplateData) (string, any) {
				subject := "New release for " + data.RepositoryFullName + ": " + data.TagName
				return subject, struct {
					Subject string
					ReleaseTemplateData
				}{
					Subject:             subject,
					ReleaseTemplateData: data,
				}
			},
		),
	}

	renderer := &TemplateRenderer{
		renderers: make(map[string]renderFunc),
	}

	for kind, spec := range specs {
		tmpl, err := template.ParseFS(templateFS, spec.path)
		if err != nil {
			return nil, err
		}

		kind := kind
		spec := spec
		renderer.renderers[kind] = func(data any) (RenderedEmail, error) {
			return spec.render(tmpl, data)
		}
	}

	return renderer, nil
}

func (r *TemplateRenderer) Render(kind string, data any) (RenderedEmail, error) {
	render, ok := r.renderers[kind]
	if !ok {
		return RenderedEmail{}, fmt.Errorf("unknown template kind: %s", kind)
	}

	return render(data)
}

func newTemplateSpec[T any](kind string, path string, buildView func(T) (string, any)) templateSpec {
	return templateSpec{
		path: path,
		render: func(tmpl *template.Template, data any) (RenderedEmail, error) {
			payload, ok := data.(T)
			if !ok {
				return RenderedEmail{}, fmt.Errorf("invalid data for %s template", kind)
			}

			subject, templatePayload := buildView(payload)

			var htmlBody bytes.Buffer
			if err := tmpl.Execute(&htmlBody, templatePayload); err != nil {
				return RenderedEmail{}, err
			}

			return RenderedEmail{
				Subject:  subject,
				HTMLBody: htmlBody.String(),
			}, nil
		},
	}
}
