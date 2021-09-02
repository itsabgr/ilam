go build -o build/linux_amd64
cd build
git add .
git commit -m release
git push
