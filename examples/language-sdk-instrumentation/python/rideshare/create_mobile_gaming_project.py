import os
import shutil

def replace_content(content, replacements):
    for old, new in replacements.items():
        content = content.replace(old, new)
    return content

def create_new_project_structure(src_root, dest_root, replacements):
    for root, dirs, files in os.walk(src_root):
        relative_path = os.path.relpath(root, src_root)
        dest_dir = os.path.join(dest_root, replace_content(relative_path, replacements))

        if not os.path.exists(dest_dir):
            os.makedirs(dest_dir)

        for file_name in files:
            src_file = os.path.join(root, file_name)
            new_file_name = replace_content(file_name, replacements)
            dest_file = os.path.join(dest_dir, new_file_name)

            with open(src_file, 'r') as f:
                content = f.read()

            new_content = replace_content(content, replacements)

            with open(dest_file, 'w') as f:
                f.write(new_content)

if __name__ == "__main__":
    # Define the source and destination paths
    src_root = "/Users/rperry2174/Desktop/projects/pyroscope/examples/language-sdk-instrumentation/python/rideshare/flask"
    dest_root = "/Users/rperry2174/Desktop/projects/pyroscope/examples/language-sdk-instrumentation/python/mobile-gaming"

    # Define the replacements (old -> new)
    replacements = {
        "rideshare": "gaming",
        "bike": "arcade",
        "scooter": "shooter",
        "car": "puzzle",
        "Bike": "Arcade",
        "Scooter": "Shooter",
        "Car": "Puzzle",
        "vehicle": "game",
        "Vehicle": "Game",
        "server": "gaming_server"
    }

    # Create the new project structure
    create_new_project_structure(src_root, dest_root, replacements)

    print(f"New project structure created at {dest_root}")
