#!/usr/bin/env node

/**
 * generate-png.mjs — HTML → PNG via Playwright
 *
 * Usage:
 *   node generate-png.mjs <input.html> <output.png>
 */

import { chromium } from 'playwright';
import { resolve, dirname } from 'path';
import { readFile } from 'fs/promises';
import { mkdirSync } from 'fs';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));

mkdirSync(resolve(__dirname, 'output'), { recursive: true });

async function generatePNG() {
  const args = process.argv.slice(2);
  let inputPath, outputPath;

  for (const arg of args) {
    if (!inputPath) {
      inputPath = arg;
    } else if (!outputPath) {
      outputPath = arg;
    }
  }

  if (!inputPath || !outputPath) {
    console.error('Usage: node generate-png.mjs <input.html> <output.png>');
    process.exit(1);
  }

  inputPath = resolve(inputPath);
  outputPath = resolve(outputPath);

  let html = await readFile(inputPath, 'utf-8');

  const fontsDir = resolve(__dirname, 'fonts');
  html = html.replace(
    /url\(['"]?\.\/fonts\//g,
    `url('file://${fontsDir}/`
  );
  html = html.replace(
    /file:\/\/([^'")]+)\.(woff2?|ttf|otf)['"]?\)/g,
    `file://$1.$2')`
  );

  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage();
    await page.setViewportSize({ width: 1200, height: 1600 });

    await page.setContent(html, {
      waitUntil: 'networkidle',
      baseURL: `file://${dirname(inputPath)}/`,
    });

    await page.evaluate(() => document.fonts.ready);

    const element = await page.$('.page');
    if (element) {
      await element.screenshot({ path: outputPath, type: 'png' });
    } else {
      await page.screenshot({ path: outputPath, fullPage: true, type: 'png' });
    }

    console.log(`✅ PNG generated: ${outputPath}`);
  } finally {
    await browser.close();
  }
}

generatePNG().catch((err) => {
  console.error('❌ PNG generation failed:', err.message);
  process.exit(1);
});
