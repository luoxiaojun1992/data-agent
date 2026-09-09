const path = require('node:path');

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  env: {
    NEXT_PUBLIC_API_URL: process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1',
  },
  webpack: (config) => {
    // SPEC-093: 强制 transformers.js 走浏览器入口。webpack 对包名解析时
    // server 端 compiler 会命中 exports 的 node condition（onnxruntime-node
    // 的 .node 二进制导致 build parse failed）。alias 优先于 exports。
    config.resolve.alias['@huggingface/transformers'] = path.resolve(
      __dirname,
      'node_modules/@huggingface/transformers/dist/transformers.web.js',
    );
    // 浏览器 bundle 不需要 onnxruntime-node（官方 Next.js 教程同款配置）。
    config.resolve.alias['onnxruntime-node$'] = false;
    // 关键修复：transformers.js v4 通过 `onnxruntime-web/webgpu` 子路径引入
    // ort。webpack 命中 exports 的 import condition → ort.*.min.mjs，其顶层
    // `import.meta.url` 被 webpack 编译成模块对象引用（`new U(module)`），
    // 运行时抛 "e.replace is not a function"。强制 alias 到 UMD 版
    // （ort.webgpu.min.js，零 import.meta，wasm 定位走 env.wasm.wasmPaths）。
    config.resolve.alias['onnxruntime-web/webgpu$'] = path.resolve(
      __dirname,
      'node_modules/onnxruntime-web/dist/ort.webgpu.min.js',
    );
    // 兜底：onnxruntime-node 的 .node 二进制即使被引用也按源文本嵌入，不 parse。
    config.module.rules.push({
      test: /\.node$/,
      type: 'asset/source',
    });
    return config;
  },
};

module.exports = nextConfig;
