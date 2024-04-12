from rest_framework.views import APIView
from rest_framework.response import Response

from ride_share.utility.utility import find_nearest_vehicle

class CarView(APIView):
    """
    View to order a car
    """

    def order_car(self, search_radius):
        find_nearest_vehicle(search_radius, "car")

    def get(self, request, format=None):
        """
        Order a car within a certain radius.
        """
        self.order_car(0.4)
        return Response(["Car view"])
