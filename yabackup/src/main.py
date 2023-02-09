# This is a sample Python script.
import datetime

# Press Shift+F10 to execute it or replace it with your code.
# Press Double Shift to search everywhere for classes, files, tool windows, actions, and settings.

import flask;
def get_posts():
    posts = []
    posts.append({"title":"Post1", "created":datetime.datetime.now()})
    posts.append({"title":"Post2", "created":datetime.datetime.now()})
    return posts

def print_hi(name):
    # Use a breakpoint in the code line below to debug your script.
    print(f'Hi, {get_posts()}')  # Press Ctrl+F8 to toggle the breakpoint.


# Press the green button in the gutter to run the script.
if __name__ == '__main__':
    print_hi('PyCharm')

# See PyCharm help at https://www.jetbrains.com/help/pycharm/


