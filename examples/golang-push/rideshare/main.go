package main

import (
	"log"
	"net/http"
	"os"

	"rideshare/bike"
	"rideshare/car"
	"rideshare/scooter"

	"github.com/pyroscope-io/client/pyroscope"
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
	appName := os.Getenv("PYROSCOPE_APPLICATION_NAME")
	if appName == "" {
		appName = "ride-sharing-app"
	}
	_, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: appName,
		ServerAddress:   serverAddress,
		AuthToken:       os.Getenv("PYROSCOPE_AUTH_TOKEN"),
		Logger:          pyroscope.StandardLogger,
		Tags:            map[string]string{"region": os.Getenv("REGION")},
	})
	if err != nil {
		log.Fatalf("error starting pyroscope profiler: %v", err)
	}

	http.HandleFunc("/", index)
	http.HandleFunc("/bike", bikeRoute)
	http.HandleFunc("/scooter", scooterRoute)
	http.HandleFunc("/car", carRoute)

	log.Fatal(http.ListenAndServe(":5000", nil))
}
