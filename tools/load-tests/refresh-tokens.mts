import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

interface Config {
  baseUrl: string;
}

interface TestData {
  jwts: string[];
  wallets: string[][];
  apiKeys: string[];
}

interface TokenResponse {
  token: string;
}

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ENV: string = process.env.ENV || "dev";
const configPath: string = path.join(__dirname, "config", `${ENV}.json`);
const config: Config = JSON.parse(fs.readFileSync(configPath, "utf8")) as Config;
const BASE_URL: string = process.env.BASE_URL || config.baseUrl;
const dataPath: string = path.join(__dirname, "test-data.json");
const data: TestData = JSON.parse(fs.readFileSync(dataPath, "utf8")) as TestData;

async function refreshTokens(): Promise<void> {
  console.log(`Refreshing tokens for ${data.apiKeys.length} merchants...`);
  for (let i = 0; i < data.apiKeys.length; i++) {
    const apiKey = data.apiKeys[i];
    const res = await fetch(`${BASE_URL}/v1/auth/token`, {
      method: "POST",
      headers: { "Authorization": `Bearer ${apiKey}` },
    });
    if (res.ok) {
      const body = (await res.json()) as TokenResponse;
      data.jwts[i] = body.token;
      if (i % 10 === 0) console.log(`Refreshed ${i}/${data.apiKeys.length}`);
    } else {
      console.error(`Failed to refresh token for merchant ${i}: ${res.status}`);
    }
  }
  fs.writeFileSync(dataPath, JSON.stringify(data, null, 2));
  console.log("Done refreshing tokens.");
}
refreshTokens();
