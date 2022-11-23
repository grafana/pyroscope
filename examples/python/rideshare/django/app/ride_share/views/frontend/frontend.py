from django.shortcuts import render
from django.core.files.storage import FileSystemStorage
from rest_framework.views import APIView
from rest_framework.response import Response

def home_page(request):
    return render(request, "home_page.html")
