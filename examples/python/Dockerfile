FROM python:3

WORKDIR /usr/src/app

COPY --from=pyroscope/pyroscope:latest /usr/bin/pyroscope /usr/bin/pyroscope
COPY main.py ./main.py
COPY util.py ./util.py
COPY __init__.py ./__init__.py
COPY a_module ./a_module
COPY b_module ./b_module

ENV PYROSCOPE_APPLICATION_NAME=simple.python.app
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040/
CMD ["pyroscope", "exec", "python", "main.py"]
