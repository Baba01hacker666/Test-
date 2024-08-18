from flask import Flask, request, render_template, send_file
import requests
import random
import string
import io

app = Flask(__name__)

def generate_image_url(description):
    # Generate a random seed
    random_seed = ''.join(random.choices(string.digits, k=8))
    formatted_description = description.replace(' ', '%20')
    return f"https://image.pollinations.ai/prompt/{formatted_description}?nologo=true&seed={random_seed}"

def download_image(url):
    response = requests.get(url)
    if response.status_code != 200:
        raise Exception(f"Error downloading image: {response.status_code}")
    return response.content

@app.route('/', methods=['GET', 'POST'])
def index():
    if request.method == 'POST':
        description = request.form.get('description')
        if not description:
            return "Description is required", 400

        try:
            image_url = generate_image_url(description)
            image_data = download_image(image_url)
            return send_file(io.BytesIO(image_data), mimetype='image/jpeg')
        except Exception as e:
            return f"Error: {str(e)}", 500

    return render_template('index.html')

if __name__ == '__main__':
    app.run(debug=True, host='0.0.0.0', port=8080)
