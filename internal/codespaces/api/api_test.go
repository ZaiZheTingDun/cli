package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
)

func generateCodespaceList(start int, end int) []*Codespace {
	codespacesList := []*Codespace{}
	for i := start; i < end; i++ {
		codespacesList = append(codespacesList, &Codespace{
			Name: fmt.Sprintf("codespace-%d", i),
		})
	}
	return codespacesList
}

func createFakeListEndpointServer(t *testing.T, initalTotal int, finalTotal int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/codespaces" {
			t.Fatal("Incorrect path")
		}

		page := 1
		if r.URL.Query().Get("page") != "" {
			page, _ = strconv.Atoi(r.URL.Query().Get("page"))
		}

		per_page := 0
		if r.URL.Query().Get("per_page") != "" {
			per_page, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
		}

		response := struct {
			Codespaces []*Codespace `json:"codespaces"`
			TotalCount int          `json:"total_count"`
		}{
			Codespaces: []*Codespace{},
			TotalCount: finalTotal,
		}

		switch page {
		case 1:
			response.Codespaces = generateCodespaceList(0, per_page)
			response.TotalCount = initalTotal
			w.Header().Set("Link", fmt.Sprintf(`<http://%[1]s/user/codespaces?page=3&per_page=%[2]d>; rel="last", <http://%[1]s/user/codespaces?page=2&per_page=%[2]d>; rel="next"`, r.Host, per_page))
		case 2:
			response.Codespaces = generateCodespaceList(per_page, per_page*2)
			response.TotalCount = finalTotal
			w.Header().Set("Link", fmt.Sprintf(`<http://%s/user/codespaces?page=3&per_page=%d>; rel="next"`, r.Host, per_page))
		case 3:
			response.Codespaces = generateCodespaceList(per_page*2, per_page*3-per_page/2)
			response.TotalCount = finalTotal
		default:
			t.Fatal("Should not check extra page")
		}

		data, _ := json.Marshal(response)
		fmt.Fprint(w, string(data))
	}))
}

func TestListCodespaces_limited(t *testing.T) {
	svr := createFakeListEndpointServer(t, 200, 200)
	defer svr.Close()

	api := API{
		githubAPI: svr.URL,
		client:    &http.Client{},
	}
	ctx := context.TODO()
	codespaces, err := api.ListCodespaces(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}

	if len(codespaces) != 200 {
		t.Fatalf("expected 200 codespace, got %d", len(codespaces))
	}
	if codespaces[0].Name != "codespace-0" {
		t.Fatalf("expected codespace-0, got %s", codespaces[0].Name)
	}
	if codespaces[199].Name != "codespace-199" {
		t.Fatalf("expected codespace-199, got %s", codespaces[0].Name)
	}
}

func TestListCodespaces_unlimited(t *testing.T) {
	svr := createFakeListEndpointServer(t, 200, 200)
	defer svr.Close()

	api := API{
		githubAPI: svr.URL,
		client:    &http.Client{},
	}
	ctx := context.TODO()
	codespaces, err := api.ListCodespaces(ctx, -1)
	if err != nil {
		t.Fatal(err)
	}

	if len(codespaces) != 250 {
		t.Fatalf("expected 250 codespace, got %d", len(codespaces))
	}
	if codespaces[0].Name != "codespace-0" {
		t.Fatalf("expected codespace-0, got %s", codespaces[0].Name)
	}
	if codespaces[249].Name != "codespace-249" {
		t.Fatalf("expected codespace-249, got %s", codespaces[0].Name)
	}
}

func TestGetRepoSuggestions(t *testing.T) {
	tests := []struct {
		searchText string // The input search string
		queryText  string // The wanted query string (based off searchText)
		sort       string // (Optional) The RepoSearchParameters.Sort param
		maxRepos   string // (Optional) The RepoSearchParameters.MaxRepos param
	}{
		{
			searchText: "test",
			queryText:  "test",
		},
		{
			searchText: "org/repo",
			queryText:  "repo user:org",
		},
		{
			searchText: "org/repo/extra",
			queryText:  "repo/extra user:org",
		},
		{
			searchText: "test",
			queryText:  "test",
			sort:       "stars",
			maxRepos:   "1000",
		},
	}

	for _, tt := range tests {
		runRepoSearchTest(t, tt.searchText, tt.queryText, tt.sort, tt.maxRepos)
	}
}

func createFakeSearchReposServer(t *testing.T, wantSearchText string, wantSort string, wantPerPage string, responseRepos []*Repository) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/repositories" {
			t.Error("Incorrect path")
			return
		}

		query := r.URL.Query()
		got := fmt.Sprintf("q=%q sort=%s per_page=%s", query.Get("q"), query.Get("sort"), query.Get("per_page"))
		want := fmt.Sprintf("q=%q sort=%s per_page=%s", wantSearchText+" in:name", wantSort, wantPerPage)
		if got != want {
			t.Errorf("for query, got %s, want %s", got, want)
			return
		}

		response := struct {
			Items []*Repository `json:"items"`
		}{
			responseRepos,
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
}

func runRepoSearchTest(t *testing.T, searchText, wantQueryText, wantSort, wantMaxRepos string) {
	wantRepoNames := []string{"repo1", "repo2"}

	apiResponseRepositories := make([]*Repository, 0)
	for _, name := range wantRepoNames {
		apiResponseRepositories = append(apiResponseRepositories, &Repository{FullName: name})
	}

	svr := createFakeSearchReposServer(t, wantQueryText, wantSort, wantMaxRepos, apiResponseRepositories)
	defer svr.Close()

	api := API{
		githubAPI: svr.URL,
		client:    &http.Client{},
	}

	ctx := context.Background()

	searchParameters := RepoSearchParameters{}
	if len(wantSort) > 0 {
		searchParameters.Sort = wantSort
	}
	if len(wantMaxRepos) > 0 {
		searchParameters.MaxRepos, _ = strconv.Atoi(wantMaxRepos)
	}

	gotRepoNames, err := api.GetCodespaceRepoSuggestions(ctx, searchText, searchParameters)
	if err != nil {
		t.Fatal(err)
	}

	gotNamesStr := fmt.Sprintf("%v", gotRepoNames)
	wantNamesStr := fmt.Sprintf("%v", wantRepoNames)
	if gotNamesStr != wantNamesStr {
		t.Fatalf("got repo names %s, want %s", gotNamesStr, wantNamesStr)
	}
}

func TestRetries(t *testing.T) {
	var callCount int
	csName := "test_codespace"
	handler := func(w http.ResponseWriter, r *http.Request) {
		if callCount == 3 {
			err := json.NewEncoder(w).Encode(Codespace{
				Name: csName,
			})
			if err != nil {
				t.Fatal(err)
			}
			return
		}
		callCount++
		w.WriteHeader(502)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler(w, r) }))
	t.Cleanup(srv.Close)
	a := &API{
		githubAPI: srv.URL,
		client:    &http.Client{},
	}
	cs, err := a.GetCodespace(context.Background(), "test", false)
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 3 {
		t.Fatalf("expected at least 2 retries but got %d", callCount)
	}
	if cs.Name != csName {
		t.Fatalf("expected codespace name to be %q but got %q", csName, cs.Name)
	}
	callCount = 0
	handler = func(w http.ResponseWriter, r *http.Request) {
		callCount++
		err := json.NewEncoder(w).Encode(Codespace{
			Name: csName,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	cs, err = a.GetCodespace(context.Background(), "test", false)
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 1 {
		t.Fatalf("expected no retries but got %d calls", callCount)
	}
	if cs.Name != csName {
		t.Fatalf("expected codespace name to be %q but got %q", csName, cs.Name)
	}
}

func TestCodespace_ExportData(t *testing.T) {
	type fields struct {
		Name        string
		CreatedAt   string
		DisplayName string
		LastUsedAt  string
		Owner       User
		Repository  Repository
		State       string
		GitStatus   CodespaceGitStatus
		Connection  CodespaceConnection
		Machine     CodespaceMachine
	}
	type args struct {
		fields []string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   map[string]interface{}
	}{
		{
			name: "just name",
			fields: fields{
				Name: "test",
			},
			args: args{
				fields: []string{"name"},
			},
			want: map[string]interface{}{
				"name": "test",
			},
		},
		{
			name: "just owner",
			fields: fields{
				Owner: User{
					Login: "test",
				},
			},
			args: args{
				fields: []string{"owner"},
			},
			want: map[string]interface{}{
				"owner": "test",
			},
		},
		{
			name: "just machine",
			fields: fields{
				Machine: CodespaceMachine{
					Name: "test",
				},
			},
			args: args{
				fields: []string{"machineName"},
			},
			want: map[string]interface{}{
				"machineName": "test",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Codespace{
				Name:        tt.fields.Name,
				CreatedAt:   tt.fields.CreatedAt,
				DisplayName: tt.fields.DisplayName,
				LastUsedAt:  tt.fields.LastUsedAt,
				Owner:       tt.fields.Owner,
				Repository:  tt.fields.Repository,
				State:       tt.fields.State,
				GitStatus:   tt.fields.GitStatus,
				Connection:  tt.fields.Connection,
				Machine:     tt.fields.Machine,
			}
			if got := c.ExportData(tt.args.fields); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Codespace.ExportData() = %v, want %v", got, tt.want)
			}
		})
	}
}

func createFakeEditServer(t *testing.T, codespaceName string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkPath := "/user/codespaces/" + codespaceName

		if r.URL.Path != checkPath {
			t.Fatal("Incorrect path")
		}

		if r.Method != http.MethodPatch {
			t.Fatal("Incorrect method")
		}

		body := r.Body
		if body == nil {
			t.Fatal("No body")
		}
		defer body.Close()

		var data map[string]interface{}
		err := json.NewDecoder(body).Decode(&data)

		if err != nil {
			t.Fatal(err)
		}

		if data["display_name"] != "changeTo" {
			t.Fatal("Incorrect display name")
		}

		response := Codespace{
			DisplayName: "changeTo",
		}

		responseData, _ := json.Marshal(response)
		fmt.Fprint(w, string(responseData))
	}))
}
func TestAPI_EditCodespace(t *testing.T) {
	type args struct {
		ctx           context.Context
		codespaceName string
		params        *EditCodespaceParams
	}
	tests := []struct {
		name    string
		args    args
		want    *Codespace
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				ctx:           context.Background(),
				codespaceName: "test",
				params: &EditCodespaceParams{
					DisplayName: "changeTo",
				},
			},
			want: &Codespace{
				DisplayName: "changeTo",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svr := createFakeEditServer(t, tt.args.codespaceName)
			defer svr.Close()

			a := &API{
				client:    &http.Client{},
				githubAPI: svr.URL,
			}
			got, err := a.EditCodespace(tt.args.ctx, tt.args.codespaceName, tt.args.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("API.EditCodespace() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("API.EditCodespace() = %v, want %v", got.DisplayName, tt.want.DisplayName)
			}
		})
	}
}
