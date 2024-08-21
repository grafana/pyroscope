from utility.utility import find_nearest_game, simulate_physics

def order_arcade(search_radius):
    simulate_physics(search_radius, "arcade")
    find_nearest_game(search_radius, "arcade")
