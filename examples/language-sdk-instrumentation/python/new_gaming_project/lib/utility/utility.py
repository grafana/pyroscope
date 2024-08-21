import pyroscope
import random
import time

def find_nearest_game(search_radius, game):
    with pyroscope.tag_wrapper({"game": game}):
        for _ in range(int(search_radius * 10**6)):
            _ = random.random() * random.random()

def simulate_physics(time_step, game):
    with pyroscope.tag_wrapper({"game": game, "task": "physics_simulation"}):
        for _ in range(int(time_step * 10**6)):
            _ = random.random() * random.random()

def solve_puzzle():
    with pyroscope.tag_wrapper({"task": "solve_puzzle"}):
        for _ in range(10**7):
            _ = random.random() * random.random()

def calculate_path(start, goal):
    with pyroscope.tag_wrapper({"task": "ai_pathfinding"}):
        for _ in range(10**7):
            _ = random.random() * random.random()
