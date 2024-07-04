"""
WSGI config for hello_django project.

It exposes the WSGI callable as a module-level variable named ``application``.

For more information on this file, see
https://docs.djangoproject.com/en/3.2/howto/deployment/wsgi/
"""

import os

from django.core.wsgi import get_wsgi_application

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'hello_django.settings')

if os.environ.get('PYROSCOPE_BENCHMARK') == 'true':
	addr = "http://127.0.0.1:4040"
	samplerate = os.environ.get('PYROSCOPE_BENCHMARK_SAMPLERATE')
	if samplerate:
		samplerate = int(samplerate)
	else:
		samplerate = 100
	print(f'using pyroscope with samplerate {samplerate}')
	print(f'running python against {addr}')
	import pyroscope
	res = pyroscope.configure(
		application_name = "django.app.opt",
		server_address = addr,
		enable_logging = False,
    sample_rate=samplerate,
	)
	
else:
	print(f'not using pyroscope')

application = get_wsgi_application()
