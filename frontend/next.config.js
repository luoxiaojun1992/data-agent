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
    // onnxruntime-web 的 bundle（ort.*.mjs）含 import.meta/ESM 语法，
    // 必须按 ESM 解析（默认被 swc 当 CJS 导致 parse failed）。
    config.module.rules.push({
      test: /node_modules\/onnxruntime-web\/.*\.mjs$/,
      type: 'javascript/esm',
    });
    config.module.rules.push({
      test: /node_modules\/@huggingface\/transformers\/.*\.mjs$/,
      type: 'javascript/esm',
    });
    // 兜底：onnxruntime-node 的 .node 二进制即使被引用也按源文本嵌入，不 parse。
    config.module.rules.push({
      test: /\.node$/,
      type: 'asset/source',
    });
    return config;
  },
};

module.exports = nextConfig;
