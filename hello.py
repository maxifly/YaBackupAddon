import datetime

from flask import Flask, render_template, request, flash, redirect, url_for

app = Flask(__name__)
app.config['SECRET_KEY'] = 'your secret key'


@app.route('/')
def index():
    posts = get_posts()
    return render_template('index.html', posts=posts)

@app.route('/<int:post_id>')
def post(post_id):
    one_post = get_post(post_id)
    return render_template('post.html', post=one_post)

@app.route('/create', methods=('GET', 'POST'))
def create():
    if request.method == 'POST':
        title = request.form['title']
        content = request.form['content']

        if not title:
            flash('Title is required!')
        else:
            return redirect(url_for('index'))
    return render_template('create.html')



def get_posts():
    posts = []
    posts.append({"id": 1, "title":"Post1", "created":datetime.datetime.now()})
    posts.append({"id": 2, "title":"Post2", "created":datetime.datetime.now()})
    return posts

def get_post(id:int):
    post = {"title": "Post title number " + str(id), "created":datetime.datetime.now(), "content":str(id) + "yyetryn ghryhry rthrthrh"}
    return post
