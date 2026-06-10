#!/usr/bin/env node
// scan-browser.mjs — Playwright-based job scanner for sites that block WebFetch
// Usage: node scan-browser.mjs

import { chromium } from 'playwright';

const POSITIVE_KEYWORDS = [
  'fp&a', 'financial analyst', 'financial planning', 'strategic finance',
  'corporate finance', 'equity research', 'investment analyst', 'research analyst',
  'management consultant', 'associate consultant', 'business analyst',
  'corporate development', 'm&a', 'valuation', 'risk analyst',
  'business development', 'strategy analyst', 'venture capital', 'private equity',
  'management trainee', 'finance associate', 'investment banking',
  'consultant', 'analyst', 'strategy', 'finance'
];

const NEGATIVE_KEYWORDS = [
  'senior manager', 'director', 'vp', 'backend', 'frontend', 'full stack',
  '.net', 'java developer', 'ios', 'android', 'php', 'ruby', 'embedded',
  'firmware', 'blockchain', 'web3', 'crypto', 'salesforce', 'sap',
  'oracle ebs', 'cobol', 'nurse', 'doctor', 'chef', 'driver',
  'accounting', 'accountant', 'tax', 'audit', 'auditor', 'payroll',
  'bookkeeper', 'actuarial', 'office boy', 'office girl', 'receptionist',
  'stock keeper', 'procurement', 'archive', 'report production'
];

const SENIORITY_BOOST = ['junior', 'associate', 'analyst', 'graduate', 'entry level', 'trainee', 'intern'];

// Companies to scan
const TARGETS = [
  {
    name: 'BCG',
    url: 'https://studenttalent.bcg.com/careerhub/explore/jobs',
    selector: '[data-automation-id="searchResultItem"], .job-listing, .job-result, a[href*="/job/"]',
    waitSelector: '[data-automation-id="searchResultItem"], .job-listing, .job-result',
  },
  {
    name: 'McKinsey',
    url: 'https://www.mckinsey.com/careers/search-jobs?query=&locations=Jakarta',
    selector: '.job-listing, .search-result, a[href*="/job/"]',
    waitSelector: '.job-listing, .search-result',
  },
  {
    name: 'Bain',
    url: 'https://www.bain.com/careers/find-a-role/?query=analyst&location=Jakarta',
    selector: '.job-listing, .search-result, a[href*="/role/"]',
    waitSelector: '.job-listing, .search-result',
  },
  {
    name: 'PwC',
    url: 'https://www.pwc.com/gx/en/careers/job-search.html?keyword=analyst&location=Jakarta',
    selector: '.job-listing, .search-result, a[href*="/job/"]',
    waitSelector: '.job-listing, .search-result',
  },
  {
    name: 'Deloitte',
    url: 'https://www2.deloitte.com/id/en/careers.html',
    selector: '.job-listing, .search-result, a[href*="/job/"]',
    waitSelector: '.job-listing, .search-result',
  },
  {
    name: 'KPMG',
    url: 'https://home.kpmg/id/en/home/careers.html',
    selector: '.job-listing, .search-result, a[href*="/job/"]',
    waitSelector: '.job-listing, .search-result',
  },
];

function matchesFilter(title) {
  const lower = title.toLowerCase();
  const hasPositive = POSITIVE_KEYWORDS.some(kw => lower.includes(kw));
  const hasNegative = NEGATIVE_KEYWORDS.some(kw => lower.includes(kw));
  const hasSeniorityBoost = SENIORITY_BOOST.some(kw => lower.includes(kw));
  return { hasPositive, hasNegative, hasSeniorityBoost, pass: hasPositive && !hasNegative };
}

async function scanTarget(browser, target) {
  const context = await browser.newContext({
    userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
  });
  const page = await context.newPage();
  const results = [];

  try {
    console.log(`\n🔍 Scanning ${target.name}: ${target.url}`);
    await page.goto(target.url, { waitUntil: 'networkidle', timeout: 30000 });

    // Try to wait for content
    try {
      await page.waitForSelector(target.waitSelector, { timeout: 10000 });
    } catch {
      console.log(`  ⚠️ No selector found, extracting all links...`);
    }

    // Extract all links with job-related text
    const links = await page.evaluate(() => {
      const allLinks = document.querySelectorAll('a[href]');
      return Array.from(allLinks).map(a => ({
        title: a.textContent.trim(),
        url: a.href,
      })).filter(l => l.title.length > 5 && l.title.length < 200);
    });

    console.log(`  Found ${links.length} links`);

    for (const link of links) {
      const filter = matchesFilter(link.title);
      if (filter.pass) {
        results.push({
          company: target.name,
          title: link.title,
          url: link.url,
          seniorityBoost: filter.hasSeniorityBoost,
        });
      }
    }

    console.log(`  ✅ ${results.length} matching roles after filter`);
  } catch (err) {
    console.log(`  ❌ Error: ${err.message}`);
  } finally {
    await context.close();
  }

  return results;
}

async function main() {
  console.log('🚀 Playwright Job Scanner');
  console.log('========================\n');

  const browser = await chromium.launch({ headless: true });
  const allResults = [];

  for (const target of TARGETS) {
    const results = await scanTarget(browser, target);
    allResults.push(...results);
  }

  await browser.close();

  // Dedup by URL
  const seen = new Set();
  const unique = allResults.filter(r => {
    if (seen.has(r.url)) return false;
    seen.add(r.url);
    return true;
  });

  // Sort: seniority boost first
  unique.sort((a, b) => (b.seniorityBoost ? 1 : 0) - (a.seniorityBoost ? 1 : 0));

  console.log('\n━━━━━━━━━━━━━━━━━━━━━━━━━━');
  console.log(`Total unique results: ${unique.length}`);
  console.log('━━━━━━━━━━━━━━━━━━━━━━━━━━\n');

  for (const r of unique) {
    const boost = r.seniorityBoost ? '⭐' : '  ';
    console.log(`${boost} ${r.company} | ${r.title}`);
    console.log(`    ${r.url}`);
  }

  // Output as JSON for further processing
  console.log('\n--- JSON ---');
  console.log(JSON.stringify(unique, null, 2));
}

main().catch(console.error);
