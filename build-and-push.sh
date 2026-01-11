#!/bin/bash
set -e

# 获取版本标签
VERSION_TAG=$(date +%Y%m%d)-$(git rev-parse --short HEAD)
IMAGE_NAME="ghcr.io/whatattitude/new-api"

echo "Building Docker image..."
echo "Tags: $IMAGE_NAME:latest and $IMAGE_NAME:$VERSION_TAG"

# 构建镜像
docker build -t $IMAGE_NAME:latest -t $IMAGE_NAME:$VERSION_TAG .

echo "Build completed successfully!"
echo ""
echo "To push the image to GitHub Container Registry, run:"
echo "  docker login ghcr.io -u YOUR_GITHUB_USERNAME -p YOUR_GITHUB_TOKEN"
echo "  docker push $IMAGE_NAME:latest"
echo "  docker push $IMAGE_NAME:$VERSION_TAG"
