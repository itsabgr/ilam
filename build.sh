go build -o build/ilam -ldflags "-s -w" -v && \
cd build && \
git add . && \
git commit -m release && \
git push
