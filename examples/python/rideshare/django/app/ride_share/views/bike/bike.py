from rest_framework.views import APIView
from rest_framework.response import Response

from ride_share.utility.utility import find_nearest_vehicle

class BikeView(APIView):
    """
    View to order a bike
    """

    def order_bike(self, search_radius):
        find_nearest_vehicle(search_radius, "bike")

    def get(self, request, format=None):
        """
        Order a bike within a certain radius.
        """
        self.order_bike(0.2)
        return Response(["Bike view"])
