package main

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

// datasetFile - путь до файла с данными. Глобальная переменная, чтобы в тестах
// можно было подменить путь и получить ошибку чтения.
var datasetFile = "dataset.xml"

// xmlUser - запись пользователя в формате dataset.xml
type xmlUser struct {
	ID        int    `xml:"id"`
	FirstName string `xml:"first_name"`
	LastName  string `xml:"last_name"`
	Age       int    `xml:"age"`
	About     string `xml:"about"`
	Gender    string `xml:"gender"`
}

// xmlUsers - корневой элемент dataset.xml
type xmlUsers struct {
	Rows []xmlUser `xml:"row"`
}

// searchError - ошибка обработки запроса с HTTP-статусом и текстом ответа
type searchError struct {
	status  int
	message string
}

// SearchServer - HTTP-обработчик поискового сервиса
func SearchServer(w http.ResponseWriter, r *http.Request) {
	req, parseErr := parseSearchRequest(r)
	if parseErr != nil {
		writeError(w, parseErr.status, parseErr.message)
		return
	}

	users, err := loadUsers(datasetFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SearchServer fatal error")
		return
	}

	writeJSON(w, searchUsers(users, req))
}

// parseSearchRequest разбирает и валидирует параметры запроса
func parseSearchRequest(r *http.Request) (SearchRequest, *searchError) {
	query := r.URL.Query()

	limit, err := strconv.Atoi(query.Get("limit"))
	if err != nil || limit < 0 {
		return SearchRequest{}, &searchError{http.StatusBadRequest, "limit must be > 0"}
	}

	offset, err := strconv.Atoi(query.Get("offset"))
	if err != nil || offset < 0 {
		return SearchRequest{}, &searchError{http.StatusBadRequest, "offset must be > 0"}
	}

	orderField := query.Get("order_field")
	if orderField == "" {
		orderField = "Name"
	}
	switch orderField {
	case "Id", "Age", "Name":
	default:
		return SearchRequest{}, &searchError{http.StatusBadRequest, ErrorBadOrderField}
	}

	orderBy, err := strconv.Atoi(query.Get("order_by"))
	if err != nil {
		return SearchRequest{}, &searchError{http.StatusBadRequest, "order_by must be integer"}
	}

	return SearchRequest{
		Limit:      limit,
		Offset:     offset,
		Query:      query.Get("query"),
		OrderField: orderField,
		OrderBy:    orderBy,
	}, nil
}

// loadUsers читает и разбирает пользователей из XML-файла
func loadUsers(path string) ([]User, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var root xmlUsers
	if err := xml.NewDecoder(file).Decode(&root); err != nil {
		return nil, err
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
	return users, nil
}

// searchUsers фильтрует, сортирует и постранично ограничивает список пользователей
func searchUsers(users []User, req SearchRequest) []User {
	filtered := make([]User, 0, len(users))
	for _, user := range users {
		if req.Query == "" || strings.Contains(user.Name, req.Query) || strings.Contains(user.About, req.Query) {
			filtered = append(filtered, user)
		}
	}

	sortUsers(filtered, req.OrderField, req.OrderBy)

	start := req.Offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + req.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end]
}

// sortUsers сортирует пользователей по полю с учётом направления
func sortUsers(users []User, field string, order int) {
	if order == OrderByAsIs {
		return
	}

	less := func(i, j int) bool {
		return compareUsers(users[i], users[j], field) < 0
	}
	if order == OrderByDesc {
		less = func(i, j int) bool {
			return compareUsers(users[i], users[j], field) > 0
		}
	}

	sort.SliceStable(users, less)
}

// compareUsers сравнивает двух пользователей по указанному полю
func compareUsers(a, b User, field string) int {
	switch field {
	case "Id":
		return a.ID - b.ID
	case "Age":
		return a.Age - b.Age
	default:
		return strings.Compare(a.Name, b.Name)
	}
}

// writeJSON отправляет ответ в виде JSON-массива пользователей
func writeJSON(w http.ResponseWriter, users []User) {
	data, err := json.Marshal(users)
	if err != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		return
	}
}

// writeError отправляет ошибку в виде JSON-объекта
func writeError(w http.ResponseWriter, status int, message string) {
	data, err := json.Marshal(SearchErrorResponse{Error: message})
	if err != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		return
	}
}
