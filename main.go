package main

import (
    "log"
    "net/http"

    "github.com/gorilla/mux"
)

func main() {
    r := mux.NewRouter()
    
    // Rutas
    r.HandleFunc("/", HomeHandler).Methods("GET")
    r.HandleFunc("/api/items", GetItemsHandler).Methods("GET")
    r.HandleFunc("/api/items", CreateItemHandler).Methods("POST")

    // Middleware
    r.Use(loggingMiddleware)

    // Iniciar servidor
    log.Println("Servidor iniciado en http://localhost:8080")
    log.Fatal(http.ListenAndServe(":8080", r))
}

func HomeHandler(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("¡Bienvenido a la API!"))
}

func GetItemsHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Write([]byte(`{"items": []}`))
}

func CreateItemHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    w.Write([]byte(`{"message": "Item creado correctamente"}`))
}

func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        log.Printf("%s %s", r.Method, r.RequestURI)
        next.ServeHTTP(w, r)
    })
}
