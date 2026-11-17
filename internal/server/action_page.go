/*
Copyright 2026-present SonoLink.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"html/template"
	"net/http"
)

// actionPage is what the action page shows: a confirmation for Action, or
// Error when the action could not run.
type actionPage struct {
	Action  string
	Repo    string
	PRURL   string
	PostURL string
	Token   string
	Error   string
}

// actionPageTemplate asks the author to confirm an action with the browser's
// confirm dialog, posts the token back when they do and returns them to the
// pull request when they do not.
var actionPageTemplate = template.Must(template.New("action").Parse(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <meta name="referrer" content="no-referrer">
    <title>Arbiterer</title>
    <style>
      :root {
        color-scheme: dark;
        font-family: system-ui, sans-serif;
      }

      body {
        display: grid;
        place-items: center;
        box-sizing: border-box;
        min-height: 100vh;
        margin: 0;
        padding: 16px;
        background: #0e1418;
        color: #eef1f3;
      }

      main {
        max-width: 420px;
      }

      a {
        color: #6cc4cc;
      }
    </style>
  </head>
  <body>
    <main>
      {{- if .Error}}
      <h1>That didn't work</h1>
      <p>{{.Error}}</p>
      {{- if .PRURL}}
      <p><a href="{{.PRURL}}">Back to the pull request</a></p>
      {{- end}}
      {{- else}}
      <noscript>
        <p>Enable JavaScript to confirm, then open the link again.</p>
        <p><a href="{{.PRURL}}">Back to the pull request</a></p>
      </noscript>
      <form id="form" method="post" action="{{.PostURL}}" hidden>
        <input type="hidden" name="token" value="{{.Token}}">
      </form>
      <script>
        const message = {{if eq .Action "delete" -}}
          "Delete your data?\n\nThis removes your Discord links in every repository and the Discord tokens Arbiterer stored for you."
        {{- else -}}
          {{printf "Unlink your Discord account?\n\nThis removes your Discord link for %s. Its checks will ask you to link again." .Repo}}
        {{- end}};

        if (confirm(message)) {
          document.getElementById("form").submit();
        } else {
          location.replace({{.PRURL}});
        }
      </script>
      {{- end}}
    </main>
  </body>
</html>
`))

// writeActionPage renders page with status.
func (s *Server) writeActionPage(w http.ResponseWriter, status int, page actionPage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	if err := actionPageTemplate.Execute(w, page); err != nil {
		s.logger.Error("rendering action page", "error", err)
	}
}
