import os

def print_tree_structure(f, root_folder, prefix=""):
    entries = os.listdir(root_folder)
    entries = sorted(entries, key=lambda x: os.path.isdir(os.path.join(root_folder, x)))
    entries_count = len(entries)

    for i, entry in enumerate(entries):
        path = os.path.join(root_folder, entry)
        connector = "├── " if i < entries_count - 1 else "└── "

        if os.path.isdir(path):
            f.write(f"{prefix}{connector}{entry}/\n")
            new_prefix = prefix + ("│   " if i < entries_count - 1 else "    ")
            print_tree_structure(f, path, new_prefix)
        else:
            f.write(f"{prefix}{connector}{entry}\n")


def print_structure_and_contents(root_folder):
    output_file = os.path.join(root_folder, "folder_structure_and_contents.txt")

    with open(output_file, 'w') as f:
        f.write("Folder structure:\n")
        print_tree_structure(f, root_folder)
        f.write("\n\nDetailed file contents:\n")

        for root, dirs, files in os.walk(root_folder):
            current_dir = os.path.relpath(root, root_folder)
            if current_dir == ".":
                current_dir = os.path.basename(root_folder)
            f.write(f"Directory: {current_dir}\n")

            for file_name in files:
                file_path = os.path.join(root, file_name)
                f.write(f"  File: {file_name}\n")
                f.write("  Content:\n")
                
                # Open and print the contents of the file
                with open(file_path, 'r') as file:
                    content = file.read()
                    f.write(content + "\n")
                
                f.write("\n")  # Add extra space between files

if __name__ == "__main__":
    root_folder = "/Users/rperry2174/Desktop/projects/pyroscope/examples/language-sdk-instrumentation/python/rideshare/flask"
    print_structure_and_contents(root_folder)
