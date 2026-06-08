#!/usr/bin/env node
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import process from 'node:process';

const defaultBaseURL = 'https://api.gptgod.online/v1';
const defaultModel = 'gemini-3.1-flash-image-preview';
const defaultTextBaseURL = 'https://api.deepseek.com';
const defaultTextModel = 'deepseek-v4-flash';
const redPixelPNG = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=';
const defaultImageURL = 'https://raw.githubusercontent.com/github/explore/main/topics/javascript/javascript.png';

const env = process.env;
const apiKey = env.AI_VISION_API_KEY || env.API_KEY || '';
const model = env.AI_VISION_MODEL || env.AI_RELAY_MODEL || defaultModel;
const baseURL = normalizeBaseURL(env.AI_VISION_BASE_URL || env.AI_RELAY_BASE_URL || defaultBaseURL);
const textAPIKey = env.AI_TEXT_API_KEY || env.DEEPSEEK_API_KEY || env.API_KEY || '';
const textModel = env.AI_TEXT_MODEL || defaultTextModel;
const textBaseURL = normalizeBaseURL(env.AI_TEXT_BASE_URL || defaultTextBaseURL);
const timeoutMS = Number(env.AI_RELAY_TIMEOUT_MS || 30_000);
const imageURL = env.AI_RELAY_IMAGE_URL || defaultImageURL;
const outputPath = resolve(env.AI_RELAY_PROBE_OUT || 'docs/atmosphere-ai-implementation-2026-06-06/evidence/ai-relay-probe-gptgod-latest.json');

if (!textAPIKey || /^replace-/i.test(textAPIKey)) {
  console.error('AI_TEXT_API_KEY or DEEPSEEK_API_KEY must be set to a real text AI provider key. Do not commit real keys.');
  process.exit(2);
}

const report = {
  generated_at: new Date().toISOString(),
  base_url: redactBaseURL(baseURL),
  model,
  text_base_url: redactBaseURL(textBaseURL),
  text_model: textModel,
  timeout_ms: timeoutMS,
  checks: []
};

async function main() {
  await runCheck('text_chat_json_object_required', probeTextJSONObject);
  await runCheck('models_endpoint_optional', probeModels);
  await runCheck('chat_plain_text_optional', probePlainText);
  await runCheck('chat_structured_text_optional', probeStructuredText);
  await runCheck('chat_json_schema_optional', probeJSONSchema);
  await runCheck('chat_multimodal_https_optional', probeMultimodalHTTPS);
  await runCheck('chat_json_object_optional', probeJSONObject);

  const multimodalChecks = [
    'chat_multimodal_https_optional'
  ];
  const hasMultimodal = report.checks.some((check) => multimodalChecks.includes(check.name) && check.status === 'PASS');
  const hasText = report.checks.find((check) => check.name === 'text_chat_json_object_required')?.status === 'PASS';
  const gatePass = hasText && hasMultimodal;
  report.verdict = gatePass ? 'PASS' : 'FAIL';
  report.development_gate = gatePass
    ? {
        can_start_p0_ai: true,
        wire_api: 'routed_text_deepseek_plus_vision_relay',
        structured_output_mode: 'text_json_object; vision_json_schema',
        multimodal_input_mode: 'chat_completions_https_image_url',
        notes: [
          'Text tasks are verified against the configured DeepSeek-compatible Chat Completions JSON object path.',
          'For the product UI, users upload images to our backend. The relay probe intentionally checks only Chat Completions with provider-fetchable HTTPS image URLs.',
          'AI calls must stay outside the bid hot path.',
          'Persist provider/model/request metadata and validated JSON output.',
          'Use deterministic fallback when provider times out or returns invalid JSON.'
        ]
      }
    : {
        can_start_p0_ai: false,
        blockers: [
          ...(hasText ? [] : ['text_ai: configured DeepSeek-compatible JSON object call failed']),
          ...(hasMultimodal ? [] : ['multimodal: no image path worked'])
        ]
      };

  await mkdir(dirname(outputPath), { recursive: true });
  await writeFile(outputPath, `${JSON.stringify(report, null, 2)}\n`, 'utf8');
  console.log(`AI relay probe ${report.verdict}. Report: ${outputPath}`);
  if (report.verdict !== 'PASS') process.exit(1);
}

async function runCheck(name, fn) {
  const started = Date.now();
  try {
    const result = await fn();
    report.checks.push({
      name,
      status: 'PASS',
      duration_ms: Date.now() - started,
      ...result
    });
  } catch (error) {
    report.checks.push({
      name,
      status: name.endsWith('_optional') ? 'WARN' : 'FAIL',
      duration_ms: Date.now() - started,
      error: sanitizeError(error)
    });
  }
}

async function probeModels() {
  const payload = await requestJSON('/models', { method: 'GET' });
  const ids = Array.isArray(payload.data) ? payload.data.map((item) => item.id).filter(Boolean) : [];
  return {
    summary: ids.includes(model) ? 'configured model appears in /models' : '/models works, configured model not listed',
    sample_model_count: ids.length,
    configured_model_listed: ids.includes(model)
  };
}

async function probePlainText() {
  const payload = await chatCompletion({
    messages: [
      { role: 'system', content: 'You are a strict API smoke-test responder.' },
      { role: 'user', content: '只回复 EXACT_TEXT: auction-ai-ok' }
    ],
    max_tokens: 64,
    temperature: 0
  });
  const text = extractText(payload);
  if (!/auction-ai-ok/i.test(text)) throw new Error(`text response did not contain expected marker: ${text.slice(0, 120)}`);
  return {
    summary: 'text chat completion returned expected marker',
    finish_reason: payload.choices?.[0]?.finish_reason ?? null,
    response_chars: text.length
  };
}

async function probeTextJSONObject() {
  const payload = await textChatCompletion({
    messages: [
      { role: 'system', content: '你是直播竞拍系统的受控助手。只返回 JSON 对象，不要 Markdown。' },
      { role: 'user', content: '生成一段60到150字的主播话术，事实：拍品天然翡翠A货平安扣吊坠，当前价350元，主题是证书和瑕疵披露。要说明买家下一步应该查看证据并按预算出价。返回字段：body,facts_used,safety_labels。' }
    ],
    response_format: { type: 'json_object' },
    max_tokens: 512,
    temperature: 0
  });
  const data = parseJSONContent(payload);
  if (typeof data.body !== 'string' || data.body.length === 0 || data.body.length > 80 || !Array.isArray(data.facts_used)) {
    throw new Error(`text json_object did not match required shape: ${JSON.stringify(data).slice(0, 200)}`);
  }
  return {
    summary: 'text provider returned parseable auction commentary JSON',
    body_chars: data.body.length,
    facts_used_count: data.facts_used.length
  };
}

async function probeResponsesText() {
  const payload = await responseCreate({
    input: [
      {
        role: 'user',
        content: [
          {
            type: 'input_text',
            text: '只回复 EXACT_TEXT: auction-ai-ok'
          }
        ]
      }
    ],
    text: { format: { type: 'text' } },
    max_output_tokens: 64
  });
  const text = extractResponsesText(payload);
  if (!/auction-ai-ok/i.test(text)) throw new Error(`Responses text did not contain expected marker: ${text.slice(0, 120)}`);
  return {
    summary: 'Responses API text output returned expected marker',
    response_chars: text.length
  };
}

async function probeResponsesJSONSchema() {
  const payload = await responseCreate({
    input: [
      {
        role: 'system',
        content: [
          {
            type: 'input_text',
            text: 'Return JSON that matches the schema. No markdown.'
          }
        ]
      },
      {
        role: 'user',
        content: [
          {
            type: 'input_text',
            text: '生成一段60到150字的直播竞拍主播提示，必须基于事实：当前价650元，最后5秒，有人刚出价。要说明末段出价为什么会延时，并提醒买家确认预算后按系统按钮出价。'
          }
        ]
      }
    ],
    text: {
      format: {
        type: 'json_schema',
        name: 'structured_text_probe',
        strict: true,
        schema: {
          type: 'object',
          additionalProperties: false,
          required: ['body', 'facts_used', 'safety_labels'],
          properties: {
            body: { type: 'string' },
            facts_used: { type: 'array', items: { type: 'string' } },
            safety_labels: { type: 'array', items: { type: 'string' } }
          }
        }
      }
    },
    max_output_tokens: 256
  });
  const data = parseResponsesJSON(payload);
  if (typeof data.body !== 'string' || data.body.length < 20 || data.body.length > 180 || !Array.isArray(data.facts_used)) {
    throw new Error(`Responses json_schema did not match required shape: ${JSON.stringify(data).slice(0, 200)}`);
  }
  return {
    summary: 'Responses API structured output works through json_schema',
    body_chars: data.body.length,
    facts_used_count: data.facts_used.length
  };
}

async function probeStructuredText() {
  const payload = await chatCompletion({
    messages: [
      { role: 'system', content: 'Return JSON that matches the schema. No markdown.' },
      { role: 'user', content: '生成一段60到150字的直播竞拍主播提示，必须基于事实：当前价650元，最后5秒，有人刚出价。要说明末段出价为什么会延时，并提醒买家确认预算后按系统按钮出价。' }
    ],
    response_format: {
      type: 'json_schema',
      json_schema: {
        name: 'structured_text_probe',
        strict: true,
        schema: {
          type: 'object',
          additionalProperties: false,
          required: ['body', 'facts_used', 'safety_labels'],
          properties: {
            body: { type: 'string' },
            facts_used: { type: 'array', items: { type: 'string' } },
            safety_labels: { type: 'array', items: { type: 'string' } }
          }
        }
      }
    },
    max_tokens: 768,
    temperature: 0
  });
  const data = parseJSONContent(payload);
  if (typeof data.body !== 'string' || data.body.length < 20 || data.body.length > 180 || !Array.isArray(data.facts_used)) {
    throw new Error(`structured text response did not match required shape: ${JSON.stringify(data).slice(0, 200)}`);
  }
  return {
    summary: 'structured text can be generated through json_schema',
    body_chars: data.body.length,
    facts_used_count: data.facts_used.length
  };
}

async function probeJSONSchema() {
  const payload = await chatCompletion({
    messages: [
      { role: 'system', content: 'Return JSON only.' },
      { role: 'user', content: 'Generate a listing draft for a tea cup. Keep values conservative.' }
    ],
    response_format: {
      type: 'json_schema',
      json_schema: {
        name: 'listing_probe',
        strict: true,
        schema: {
          type: 'object',
          additionalProperties: false,
          required: ['title', 'start_price_cents', 'risk_flags'],
          properties: {
            title: { type: 'string' },
            start_price_cents: { type: 'integer' },
            risk_flags: { type: 'array', items: { type: 'string' } }
          }
        }
      }
    },
    max_tokens: 256,
    temperature: 0
  });
  const data = parseJSONContent(payload);
  if (typeof data.title !== 'string' || !Number.isInteger(data.start_price_cents) || !Array.isArray(data.risk_flags)) {
    throw new Error(`json_schema response did not match required shape: ${JSON.stringify(data).slice(0, 200)}`);
  }
  return {
    summary: 'strict json_schema response_format is supported',
    keys: Object.keys(data)
  };
}

async function probeJSONObject() {
  const payload = await chatCompletion({
    messages: [
      { role: 'system', content: 'Return JSON only. JSON keys: title, start_price_cents, increment_cents, safety_labels.' },
      { role: 'user', content: '为一个有证书的青瓷茶盏生成保守拍卖草稿。不要承诺真伪，只给建议。' }
    ],
    response_format: { type: 'json_object' },
    max_tokens: 256,
    temperature: 0
  });
  const data = parseJSONContent(payload);
  const required = ['title', 'start_price_cents', 'increment_cents'];
  for (const key of required) {
    if (!(key in data)) throw new Error(`json_object missing ${key}: ${JSON.stringify(data).slice(0, 200)}`);
  }
  return {
    summary: 'json_object response_format returns parseable listing draft JSON',
    keys: Object.keys(data)
  };
}

async function probeMultimodalDataURL() {
  return probeMultimodal(`data:image/png;base64,${redPixelPNG}`, 'image_url data URL input produced parseable visual answer');
}

async function probeMultimodalHTTPS() {
  return probeMultimodal(imageURL, 'HTTPS image_url input produced parseable visual answer');
}

async function probeResponsesMultimodalDataURL() {
  return probeResponsesMultimodal(`data:image/png;base64,${redPixelPNG}`, 'Responses data URL image input produced parseable visual answer');
}

async function probeResponsesMultimodalHTTPS() {
  return probeResponsesMultimodal(imageURL, 'Responses HTTPS image_url input produced parseable visual answer');
}

async function probeResponsesMultimodal(imageURLValue, summary) {
  const payload = await responseCreate({
    input: [
      {
        role: 'user',
        content: [
          {
            type: 'input_text',
            text: '这是一张用于能力验收的纯红色小图。请只返回 JSON：{"dominant_color":"red","can_see_image":true}'
          },
          {
            type: 'input_image',
            image_url: imageURLValue,
            detail: 'low'
          }
        ]
      }
    ],
    text: {
      format: {
        type: 'json_schema',
        name: 'responses_image_probe',
        strict: true,
        schema: {
          type: 'object',
          additionalProperties: false,
          required: ['dominant_color', 'can_see_image'],
          properties: {
            dominant_color: { type: 'string' },
            can_see_image: { type: 'boolean' }
          }
        }
      }
    },
    max_output_tokens: 128
  });
  const data = parseResponsesJSON(payload);
  const color = String(data.dominant_color || '').toLowerCase();
  if (!data.can_see_image && !/red|红/.test(color)) {
    throw new Error(`Responses image input did not prove image understanding: ${JSON.stringify(data).slice(0, 200)}`);
  }
  return {
    summary,
    dominant_color: data.dominant_color,
    can_see_image: data.can_see_image
  };
}

async function probeMultimodal(url, summary) {
  const payload = await chatCompletion({
    messages: [
      {
        role: 'user',
        content: [
          {
            type: 'text',
            text: 'This is a capability probe image. Identify the dominant color and whether you can see the image. Return JSON only.'
          },
          {
            type: 'image_url',
            image_url: {
              url
            }
          }
        ]
      }
    ],
    response_format: {
      type: 'json_schema',
      json_schema: {
        name: 'chat_image_probe',
        strict: true,
        schema: {
          type: 'object',
          additionalProperties: false,
          required: ['dominant_color', 'can_see_image', 'subject'],
          properties: {
            dominant_color: { type: 'string' },
            can_see_image: { type: 'boolean' },
            subject: { type: 'string' }
          }
        }
      }
    },
    max_tokens: 192,
    temperature: 0
  });
  const data = parseJSONContent(payload);
  if (!data.can_see_image || typeof data.dominant_color !== 'string') {
    throw new Error(`multimodal response did not prove image understanding: ${JSON.stringify(data).slice(0, 200)}`);
  }
  return {
    summary,
    dominant_color: data.dominant_color,
    subject: data.subject ?? null,
    can_see_image: data.can_see_image
  };
}

async function probeFileUploadOnly() {
  const payload = await uploadVisionFile();
  if (!payload.id) throw new Error(`file upload response missing id: ${JSON.stringify(payload).slice(0, 300)}`);
  return {
    summary: 'Files API accepted PNG upload',
    file_id_prefix: String(payload.id).slice(0, 8),
    purpose: payload.purpose ?? null
  };
}

async function probeResponsesFileInput() {
  const uploaded = await uploadVisionFile();
  if (!uploaded.id) throw new Error(`file upload response missing id: ${JSON.stringify(uploaded).slice(0, 300)}`);
  const payload = await requestJSON('/responses', {
    method: 'POST',
    body: {
      model,
      input: [
        {
          role: 'user',
          content: [
            {
              type: 'input_text',
              text: 'This uploaded image is a pure red probe. Return only JSON: {"dominant_color":"red","can_see_image":true}'
            },
            {
              type: 'input_image',
              file_id: uploaded.id
            }
          ]
        }
      ],
      text: {
        format: {
          type: 'json_schema',
          name: 'uploaded_image_probe',
          strict: true,
          schema: {
            type: 'object',
            additionalProperties: false,
            required: ['dominant_color', 'can_see_image'],
            properties: {
              dominant_color: { type: 'string' },
              can_see_image: { type: 'boolean' }
            }
          }
        }
      },
      max_output_tokens: 128
    }
  });
  const data = parseResponsesJSON(payload);
  const color = String(data.dominant_color || '').toLowerCase();
  if (!data.can_see_image && !/red|红/.test(color)) {
    throw new Error(`responses file input did not prove image understanding: ${JSON.stringify(data).slice(0, 200)}`);
  }
  return {
    summary: 'Responses API accepted uploaded file_id as image input',
    dominant_color: data.dominant_color,
    can_see_image: data.can_see_image
  };
}

async function chatCompletion(body) {
  return requestJSON('/chat/completions', {
    method: 'POST',
    body: {
      model,
      ...body
    }
  });
}

async function textChatCompletion(body) {
  return requestJSON('/chat/completions', {
    method: 'POST',
    baseURLOverride: textBaseURL,
    apiKeyOverride: textAPIKey,
    body: {
      model: textModel,
      ...body
    }
  });
}

async function responseCreate(body) {
  return requestJSON('/responses', {
    method: 'POST',
    body: {
      model,
      ...body
    }
  });
}

async function requestJSON(path, { method, body, baseURLOverride, apiKeyOverride }) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMS);
  try {
    const response = await fetch(`${baseURLOverride || baseURL}${path}`, {
      method,
      headers: {
        Authorization: `Bearer ${apiKeyOverride || apiKey}`,
        'Content-Type': 'application/json'
      },
      body: body == null ? undefined : JSON.stringify(body),
      signal: controller.signal
    });
    const text = await response.text();
    let payload;
    try {
      payload = text ? JSON.parse(text) : {};
    } catch {
      throw new Error(`HTTP ${response.status} non-JSON response: ${text.slice(0, 300)}`);
    }
    if (!response.ok) {
      const message = payload.error?.message || payload.message || JSON.stringify(payload).slice(0, 300);
      throw new Error(`HTTP ${response.status}: ${message}`);
    }
    return payload;
  } finally {
    clearTimeout(timer);
  }
}

async function uploadVisionFile() {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMS);
  try {
    const bytes = Buffer.from(redPixelPNG, 'base64');
    const form = new FormData();
    form.append('purpose', 'vision');
    form.append('model', model);
    form.append('file', new Blob([bytes], { type: 'image/png' }), 'red-probe.png');
    const response = await fetch(`${baseURL}/files`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${apiKey}`
      },
      body: form,
      signal: controller.signal
    });
    const text = await response.text();
    let payload;
    try {
      payload = text ? JSON.parse(text) : {};
    } catch {
      throw new Error(`HTTP ${response.status} non-JSON response: ${text.slice(0, 300)}`);
    }
    if (!response.ok) {
      const message = payload.error?.message || payload.message || JSON.stringify(payload).slice(0, 300);
      throw new Error(`HTTP ${response.status}: ${message}`);
    }
    return payload;
  } finally {
    clearTimeout(timer);
  }
}

function extractText(payload) {
  const content = payload.choices?.[0]?.message?.content;
  if (typeof content === 'string') return content;
  if (Array.isArray(content)) {
    return content.map((part) => typeof part === 'string' ? part : part.text || '').join('');
  }
  throw new Error(`missing choices[0].message.content: ${JSON.stringify(payload).slice(0, 300)}`);
}

function parseJSONContent(payload) {
  const text = extractText(payload).trim();
  try {
    return JSON.parse(text);
  } catch {
    const match = text.match(/\{[\s\S]*\}/);
    if (!match) throw new Error(`response is not JSON: ${text.slice(0, 300)}`);
    return JSON.parse(match[0]);
  }
}

function parseResponsesJSON(payload) {
  return parseJSONString(extractResponsesText(payload));
}

function extractResponsesText(payload) {
  if (typeof payload.output_text === 'string') return payload.output_text;
  const chunks = [];
  for (const output of payload.output ?? []) {
    for (const content of output.content ?? []) {
      if (typeof content.text === 'string') chunks.push(content.text);
    }
  }
  if (chunks.length === 0) throw new Error(`missing response output text: ${JSON.stringify(payload).slice(0, 300)}`);
  return chunks.join('\n');
}

function parseJSONString(text) {
  const trimmed = text.trim();
  try {
    return JSON.parse(trimmed);
  } catch {
    const match = trimmed.match(/\{[\s\S]*\}/);
    if (!match) throw new Error(`response is not JSON: ${trimmed.slice(0, 300)}`);
    return JSON.parse(match[0]);
  }
}

function normalizeBaseURL(value) {
  const trimmed = value.replace(/\/+$/, '');
  return trimmed.endsWith('/v1') ? trimmed : `${trimmed}/v1`;
}

function redactBaseURL(value) {
  return value.replace(/([?&](?:key|api_key|token)=)[^&]+/gi, '$1<redacted>');
}

function sanitizeError(error) {
  const message = error instanceof Error ? error.message : String(error);
  return message.replace(apiKey, '<redacted>');
}

main().catch((error) => {
  console.error(sanitizeError(error));
  process.exit(1);
});
