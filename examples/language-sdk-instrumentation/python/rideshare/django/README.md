# Dockerizing Django with Pyroscope, Postgres, Gunicorn, and Nginx
This is a simple rideshare example that adds Pyroscope to a Django application and uses it to profile various routes

### Development
Uses the default Django development server.

1. Rename *.env.dev-sample* to *.env.dev*.
1. Update the environment variables in the *docker-compose.yml* and *.env.dev* files.
1. Build the images and run the containers:

    ```sh
    $ docker-compose up -d --build
    ```

    Test it out at [http://localhost:8000](http://localhost:8000). The "app" folder is mounted into the container and your code changes apply automatically.
