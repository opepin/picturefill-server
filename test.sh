docker build -t img-srv . && docker run -it --publish 6767:8787 --rm --name img-srv-app --env ORIGIN_SERVER=https://c4.staticflickr.com img-srv
