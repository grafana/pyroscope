FROM --platform=linux/amd64 python:3.10 AS builder

# Install build-base to allow for compilation of the profiling agent.
RUN apt-get update && apt-get install -y build-essential git

# Compile the profiling agent, generating wheels for it.
COPY requirements.txt .
RUN pip3 wheel --wheel-dir=/tmp/wheels -r requirements.txt

FROM --platform=linux/amd64 python:3.10

# Copy over the directory containing wheels for the profiling agent.
COPY --from=builder /tmp/wheels /tmp/wheels
RUN ls -al

RUN apt-get update && apt-get install -y git

# Install the profiling agent.
COPY requirements.txt .
COPY main.py .

RUN pip3 install --no-index --find-links=/tmp/wheels -r requirements.txt

EXPOSE 8080
# Run the application when the docker image is run, using either CMD (as is done
# here) or ENTRYPOINT.
CMD python3 -u main.py
