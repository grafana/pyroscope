package main

import (
	"net/http"
	"os"

	"github.com/pyroscope-io/client/pyroscope"
	"github.com/pyroscope-io/pyroscope/tree/main/examples/golang/bike"
	"github.com/pyroscope-io/pyroscope/tree/main/examples/golang/car"
	"github.com/pyroscope-io/pyroscope/tree/main/examples/golang/scooter"
)

func bikeRoute(w http.ResponseWriter, r *http.Request) {
	bike.OrderBike(1)
	w.Write([]byte("<h1>Bike ordered</h1>"))
}

func scooterRoute(w http.ResponseWriter, r *http.Request) {
	scooter.OrderScooter(2)
	w.Write([]byte("<h1>Scooter ordered</h1>"))
}

func carRoute(w http.ResponseWriter, r *http.Request) {
	car.OrderCar(3)
	w.Write([]byte("<h1>Car ordered</h1>"))
}

func index(w http.ResponseWriter, r *http.Request) {
	result := "<h1>environment vars:</h1>"
	for _, env := range os.Environ() {
		result += env + "<br>"
	}
	w.Write([]byte(result))
}

func main() {
	serverAddress := os.Getenv("PYROSCOPE_SERVER_ADDRESS")
	if serverAddress == "" {
		serverAddress = "http://localhost:4040"
	}
	pyroscope.Start(pyroscope.Config{
		ApplicationName: "ride-sharing-app",
		ServerAddress:   serverAddress,
		Logger:          pyroscope.StandardLogger,
		Tags:            map[string]string{"region": os.Getenv("REGION")},
	})

	http.HandleFunc("/", index)
	http.HandleFunc("/bike", bikeRoute)
	http.HandleFunc("/scooter", scooterRoute)
	http.HandleFunc("/car", carRoute)
	http.ListenAndServe(":5000", nil)
}
