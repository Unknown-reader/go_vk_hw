package main

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	orderFieldName = "Name"
	orderFieldAge  = "Age"
	errLimit       = "limit must be > 0"
)

// datasetUsers читает реальный dataset.xml и возвращает полный список пользователей.
func datasetUsers(t *testing.T) []User {
	t.Helper()

	file, err := os.Open(datasetFile)
	if err != nil {
		t.Fatalf("не удалось открыть %s: %v", datasetFile, err)
	}
	defer file.Close()

	var root xmlUsers
	if err := xml.NewDecoder(file).Decode(&root); err != nil {
		t.Fatalf("не удалось разобрать %s: %v", datasetFile, err)
	}

	users := make([]User, 0, len(root.Rows))
	for _, row := range root.Rows {
		users = append(users, User{
			ID:     row.ID,
			Name:   row.FirstName + " " + row.LastName,
			Age:    row.Age,
			About:  row.About,
			Gender: row.Gender,
		})
	}
	return users
}

// usersByIDs возвращает полные записи пользователей по их идентификаторам.
func usersByIDs(t *testing.T, ids ...int) []User {
	t.Helper()

	byID := make(map[int]User)
	for _, user := range datasetUsers(t) {
		byID[user.ID] = user
	}

	result := make([]User, 0, len(ids))
	for _, id := range ids {
		user, ok := byID[id]
		if !ok {
			t.Fatalf("в датасете нет пользователя с id=%d", id)
		}
		result = append(result, user)
	}
	return result
}

// TestFindUsersSearch проверяет поиск, сортировку и постраничную выдачу через SearchServer.
func TestFindUsersSearch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(SearchServer))
	defer ts.Close()

	cases := []struct {
		name     string
		req      SearchRequest
		expected []int
		nextPage bool
	}{
		{
			name:     "пустой query, сортировка по имени по возрастанию",
			req:      SearchRequest{Limit: 5, Offset: 0, OrderField: orderFieldName, OrderBy: OrderByAsc},
			expected: []int{15, 16, 19, 22, 5},
			nextPage: true,
		},
		{
			name:     "query on, сортировка по возрасту по убыванию",
			req:      SearchRequest{Limit: 1, Offset: 0, Query: "on", OrderField: orderFieldAge, OrderBy: OrderByDesc},
			expected: []int{13},
			nextPage: true,
		},
		{
			name:     "query on, сортировка по возрасту по возрастанию",
			req:      SearchRequest{Limit: 3, Offset: 0, Query: "on", OrderField: orderFieldAge, OrderBy: OrderByAsc},
			expected: []int{1, 15, 0},
			nextPage: true,
		},
		{
			name:     "query on, смещение страницы",
			req:      SearchRequest{Limit: 2, Offset: 3, Query: "on", OrderField: orderFieldAge, OrderBy: OrderByAsc},
			expected: []int{14, 2},
			nextPage: true,
		},
		{
			name:     "query on, без сортировки",
			req:      SearchRequest{Limit: 3, Offset: 0, Query: "on", OrderField: orderFieldAge, OrderBy: OrderByAsIs},
			expected: []int{0, 1, 2},
			nextPage: true,
		},
		{
			name:     "поиск по имени",
			req:      SearchRequest{Limit: 1, Offset: 0, Query: "Boyd", OrderField: orderFieldName, OrderBy: OrderByAsc},
			expected: []int{0},
			nextPage: false,
		},
		{
			name:     "поиск без совпадений",
			req:      SearchRequest{Limit: 5, Offset: 0, Query: "zzz", OrderField: orderFieldName, OrderBy: OrderByAsc},
			expected: []int{},
			nextPage: false,
		},
		{
			name:     "пустой query, сортировка по имени по убыванию",
			req:      SearchRequest{Limit: 3, Offset: 0, OrderField: orderFieldName, OrderBy: OrderByDesc},
			expected: []int{13, 33, 18},
			nextPage: true,
		},
		{
			name:     "пустой query, сортировка по Id",
			req:      SearchRequest{Limit: 3, Offset: 0, OrderField: "Id", OrderBy: OrderByAsc},
			expected: []int{0, 1, 2},
			nextPage: true,
		},
		{
			name:     "offset за пределами списка",
			req:      SearchRequest{Limit: 5, Offset: 100, Query: "on", OrderField: orderFieldName, OrderBy: OrderByAsc},
			expected: []int{},
			nextPage: false,
		},
		{
			name:     "limit больше количества совпадений",
			req:      SearchRequest{Limit: 10, Offset: 0, Query: "Boyd", OrderField: orderFieldName, OrderBy: OrderByAsc},
			expected: []int{0},
			nextPage: false,
		},
		{
			name:     "limit обрезается до 25",
			req:      SearchRequest{Limit: 100, Offset: 0, OrderField: orderFieldName, OrderBy: OrderByAsIs},
			expected: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24},
			nextPage: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &SearchClient{URL: ts.URL}
			resp, err := client.FindUsers(tc.req)
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}

			expected := usersByIDs(t, tc.expected...)
			if !reflect.DeepEqual(expected, resp.Users) {
				t.Errorf("неверный список пользователей\n\tожидалось: %#v\n\tполучено:  %#v", expected, resp.Users)
			}
			if resp.NextPage != tc.nextPage {
				t.Errorf("неверный NextPage: ожидалось %v, получено %v", tc.nextPage, resp.NextPage)
			}
		})
	}
}

// TestFindUsersExample проверяет пример из условия с жёстко заданной ожидаемой записью.
func TestFindUsersExample(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(SearchServer))
	defer ts.Close()

	client := &SearchClient{URL: ts.URL}
	resp, err := client.FindUsers(SearchRequest{
		Limit:      1,
		Offset:     0,
		Query:      "on",
		OrderField: orderFieldAge,
		OrderBy:    OrderByDesc,
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	expected := []User{{
		ID:     13,
		Name:   "Whitley Davidson",
		Age:    40,
		About:  "Consectetur dolore anim veniam aliqua deserunt officia eu. Et ullamco commodo ad officia duis ex incididunt proident consequat nostrud proident quis tempor. Sunt magna ad excepteur eu sint aliqua eiusmod deserunt proident. Do labore est dolore voluptate ullamco est dolore excepteur magna duis quis. Quis laborum deserunt ipsum velit occaecat est laborum enim aute. Officia dolore sit voluptate quis mollit veniam. Laborum nisi ullamco nisi sit nulla cillum et id nisi.\n",
		Gender: "male",
	}}
	if !reflect.DeepEqual(expected, resp.Users) {
		t.Errorf("неверный результат\n\tожидалось: %#v\n\tполучено:  %#v", expected, resp.Users)
	}
}

// TestFindUsersClientValidation проверяет ошибки, которые клиент отлавливает сам.
func TestFindUsersClientValidation(t *testing.T) {
	client := &SearchClient{URL: "http://example.com"}

	cases := []struct {
		name    string
		req     SearchRequest
		wantErr string
	}{
		{"отрицательный limit", SearchRequest{Limit: -1}, errLimit},
		{"отрицательный offset", SearchRequest{Offset: -1}, "offset must be > 0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.FindUsers(tc.req)
			if err == nil {
				t.Fatalf("ожидалась ошибка, получено nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("неверная ошибка: ожидалось %q, получено %q", tc.wantErr, err.Error())
			}
		})
	}
}

// TestFindUsersServerErrors проверяет обработку ошибок от внешней системы.
func TestFindUsersServerErrors(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{
			name: "невалидный токен",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: "bad AccessToken",
		},
		{
			name: "фатальная ошибка сервера",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: "SearchServer fatal error",
		},
		{
			name: "неизвестная ошибка bad request",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				if _, err := io.WriteString(w, `{"Error":"unknown"}`); err != nil {
					return
				}
			},
			wantErr: "unknown bad request error: unknown",
		},
		{
			name: "битый json ответа",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				if _, err := io.WriteString(w, `not json`); err != nil {
					return
				}
			},
			wantErr: "cant unpack result json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.handler)
			defer ts.Close()

			client := &SearchClient{URL: ts.URL}
			_, err := client.FindUsers(SearchRequest{Limit: 1})
			if err == nil {
				t.Fatalf("ожидалась ошибка, получено nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("неверная ошибка: ожидалось %q, получено %q", tc.wantErr, err.Error())
			}
		})
	}
}

// TestFindUsersBadOrderField проверяет ошибку невалидного поля сортировки от SearchServer.
func TestFindUsersBadOrderField(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(SearchServer))
	defer ts.Close()

	client := &SearchClient{URL: ts.URL}
	_, err := client.FindUsers(SearchRequest{Limit: 1, OrderField: "Bad"})
	if err == nil {
		t.Fatalf("ожидалась ошибка, получено nil")
	}
	if !strings.Contains(err.Error(), "OrderFeld Bad invalid") {
		t.Errorf("неверная ошибка: получено %q", err.Error())
	}
}

// TestFindUsersTimeout проверяет таймаут обращения к внешней системе.
func TestFindUsersTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer ts.Close()

	client := &SearchClient{URL: ts.URL}
	_, err := client.FindUsers(SearchRequest{Limit: 1})
	if err == nil {
		t.Fatalf("ожидалась ошибка, получено nil")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("неверная ошибка: получено %q", err.Error())
	}
}

// TestFindUsersUnknownError проверяет неизвестную ошибку сети.
func TestFindUsersUnknownError(t *testing.T) {
	client := &SearchClient{URL: "http://127.0.0.1:1"}
	_, err := client.FindUsers(SearchRequest{Limit: 1})
	if err == nil {
		t.Fatalf("ожидалась ошибка, получено nil")
	}
	if !strings.Contains(err.Error(), "unknown error") {
		t.Errorf("неверная ошибка: получено %q", err.Error())
	}
}

// TestSearchServerParams проверяет валидацию входных параметров SearchServer.
func TestSearchServerParams(t *testing.T) {
	cases := []struct {
		name       string
		url        string
		wantStatus int
		wantErr    string
	}{
		{
			name:       "невалидный limit",
			url:        "/?limit=abc&offset=0&order_by=0",
			wantStatus: http.StatusBadRequest,
			wantErr:    errLimit,
		},
		{
			name:       "отрицательный limit",
			url:        "/?limit=-1&offset=0&order_by=0",
			wantStatus: http.StatusBadRequest,
			wantErr:    errLimit,
		},
		{
			name:       "отрицательный offset",
			url:        "/?limit=1&offset=-1&order_by=0",
			wantStatus: http.StatusBadRequest,
			wantErr:    "offset must be > 0",
		},
		{
			name:       "невалидный order_field",
			url:        "/?limit=1&offset=0&order_field=Bad&order_by=0",
			wantStatus: http.StatusBadRequest,
			wantErr:    ErrorBadOrderField,
		},
		{
			name:       "невалидный order_by",
			url:        "/?limit=1&offset=0&order_by=abc",
			wantStatus: http.StatusBadRequest,
			wantErr:    "order_by must be integer",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rec := httptest.NewRecorder()

			SearchServer(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("неверный статус: ожидалось %d, получено %d", tc.wantStatus, rec.Code)
			}
			var errResp SearchErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("не удалось разобрать ответ: %v", err)
			}
			if errResp.Error != tc.wantErr {
				t.Errorf("неверная ошибка: ожидалось %q, получено %q", tc.wantErr, errResp.Error)
			}
		})
	}
}

// TestSearchServerFileErrors проверяет ошибки чтения и разбора файла с данными.
func TestSearchServerFileErrors(t *testing.T) {
	old := datasetFile
	defer func() { datasetFile = old }()

	t.Run("несуществующий файл", func(t *testing.T) {
		datasetFile = "not-exist.xml"
		req := httptest.NewRequest(http.MethodGet, "/?limit=1&offset=0&order_by=0", nil)
		rec := httptest.NewRecorder()

		SearchServer(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("неверный статус: ожидалось %d, получено %d", http.StatusInternalServerError, rec.Code)
		}
	})

	t.Run("битый xml", func(t *testing.T) {
		broken := filepath.Join(t.TempDir(), "broken.xml")
		if err := os.WriteFile(broken, []byte("<root><row>"), 0600); err != nil {
			t.Fatalf("не удалось создать файл: %v", err)
		}
		datasetFile = broken
		req := httptest.NewRequest(http.MethodGet, "/?limit=1&offset=0&order_by=0", nil)
		rec := httptest.NewRecorder()

		SearchServer(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("неверный статус: ожидалось %d, получено %d", http.StatusInternalServerError, rec.Code)
		}
	})
}
