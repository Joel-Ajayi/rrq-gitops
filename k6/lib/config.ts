const devConfig = JSON.parse(open('../config/dev.json'));
const prodConfig = JSON.parse(open('../config/prod.json'));
const thresholdsConfig = JSON.parse(open('../config/thresholds.json'));

import { ConstantArrivalRateScenario, RampingArrivalRateScenario } from 'k6/options';

export interface WorkloadsConfig {
  smoke: ConstantArrivalRateScenario;
  breakpoint: RampingArrivalRateScenario;
  load: RampingArrivalRateScenario;
  stress: RampingArrivalRateScenario;
  soak: ConstantArrivalRateScenario;
  spike: RampingArrivalRateScenario;
  chaos: ConstantArrivalRateScenario;
}

export interface EnvironmentConfig {
  environment: string;
  baseUrl: string;
  merchantCount: number;
  walletsPerMerchant: number;
  workloads: WorkloadsConfig;
  thresholds: typeof thresholdsConfig;
}

const ENV_NAME = __ENV.ENV || 'dev';
const targetConfig = ENV_NAME === 'prod' ? prodConfig : devConfig;

export const CONFIG: EnvironmentConfig = {
  environment: ENV_NAME,
  baseUrl: __ENV.BASE_URL || targetConfig.baseUrl,
  merchantCount: 100,
  walletsPerMerchant: 1000,
  workloads: targetConfig.workloads,
  thresholds: thresholdsConfig,
};
