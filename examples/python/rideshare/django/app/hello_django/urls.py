from django.contrib import admin
from django.urls import path
from django.conf import settings
from django.conf.urls.static import static

from ride_share.views.frontend.frontend import home_page
from ride_share.views.bike.bike import BikeView
from ride_share.views.car.car import CarView
from ride_share.views.scooter.scooter import ScooterView

urlpatterns = [
    path("", home_page, name="home_page"),
    path("bike/", BikeView.as_view()),
    path("car/", CarView.as_view()),
    path("scooter/", ScooterView.as_view()),
    path("admin/", admin.site.urls),
]

# urlpatterns += router.urls

if bool(settings.DEBUG):
    urlpatterns += static(settings.MEDIA_URL, document_root=settings.MEDIA_ROOT)
