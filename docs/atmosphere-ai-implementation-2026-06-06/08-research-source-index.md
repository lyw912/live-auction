# Research Source Index

Sources used to calibrate this plan. Re-check time-sensitive API/model details when implementing.

## Live Auction And Marketplace Practice

- TikTok Shop Academy, "Countdown Bidding": `https://seller-us.tiktok.com/university/essay?knowledge_id=8427133325330222&lang=en`
  Key points: Countdown Bidding is a real TikTok Shop live auction feature; sellers can configure auction duration and extended timer; last-second bids can reset the timer to a preselected extension.

- TikTok Shop Academy, "Seller Terms and Conditions for Countdown Bidding - US": `https://seller-us.tiktok.com/university/essay?knowledge_id=162003261359915`
  Key points: fixed vs extended auctions, highest bidder wins, seller fulfillment obligations, seller cannot alter auction terms after start.

- TikTok Shop Academy, "Auction Requirements": `https://seller-us.tiktok.com/university/essay?knowledge_id=6110392477746958&lang=en`
  Key points: auction access, product/listing requirements, pricing limits, permitted practices, enforcement.

- TikTok Shop Academy, "Collectibles Requirements": `https://seller-us.tiktok.com/university/essay?knowledge_id=2274231656367918&lang=en`
  Key points: collectibles require real images, accurate age/material/condition description, and clear distinction from reproductions.

- Whatnot Community Guidelines: `https://help.whatnot.com/hc/en-us/articles/360061197472-Whatnot-Community-Guidelines`
  Key points: shill bidding and troll bidding are prohibited; platform monitors bids and transactions and takes action on suspected abuse.

- Whatnot Listing Guidelines: `https://help.whatnot.com/hc/en-us/articles/360061195612-Whatnot-Listing-Guidelines`
  Key points: actual item photos, accurate title/description, no misleading/clickbait listings.

## AI Listing And Ecommerce Assistants

- eBay, "'Magical' Listing Tool Harnesses the Power of AI": `https://innovation.ebayinc.com/stories/magical-listing-tool-harnesses-the-power-of-ai-to-make-selling-on-ebay-faster-easier-and-more-accurate`
  Key points: AI listing generation from small seller input/photo; about 30% of US app sellers tried the feature on a given day and over 95% who tried used AI descriptions, with edits included.

- ZDNET, eBay image-based AI listing summary: `https://www.zdnet.com/article/ebays-new-magical-ai-tool-writes-product-descriptions-for-you-from-a-single-photo`
  Key points: seller cold-start friction is a real product problem.

- Value Added Resource, seller criticism of AI listing quality: `https://www.valueaddedresource.net/ebay-teases-magical-ai-listing-creation-using-image`
  Key points: generated descriptions can be misleading or overgeneral; human review and evidence-gated claims are necessary.

## Dark Patterns And Compliance

- FTC press release, "FTC Report Shows Rise in Sophisticated Dark Patterns": `https://www.ftc.gov/news-events/news/press-releases/2022/09/ftc-report-shows-rise-sophisticated-dark-patterns-designed-trick-trap-consumers`
  Key points: regulators scrutinize manipulative design patterns in ecommerce and digital products.

- OECD, "Dark commercial patterns": `https://www.oecd.org/content/dam/oecd/en/publications/reports/2022/10/dark-commercial-patterns_9f6169cd/44f5e846-en.pdf`
  Key points: social proof and urgency are recognized dark-pattern categories when misleading or manipulative.

- Reed Smith, "Dark patterns lead to enforcement spotlight": `https://www.reedsmith.com/articles/dark-patterns-lead-to-enforcement-spotlight-key-compliance-steps-for-businesses`
  Key points: examples include false countdown timers and false claims that others are looking at or recently bought products.

## Animation And Frontend Performance

- MDN, CSS and JavaScript animation performance: `https://developer.mozilla.org/en-US/docs/Web/Performance/Guides/CSS_JavaScript_animation_performance`
  Key points: animation performance depends on avoiding expensive layout/paint work; requestAnimationFrame is the standard browser animation mechanism.

- web.dev, high-performance animations: `https://web.dev/articles/animations-guide`
  Key points: prefer transform and opacity for smooth animations.

## Live Commerce And Mobile UX Structure

- Nielsen Norman Group, "Livestream Ecommerce: 7 Tips for Good UX": `https://www.nngroup.com/articles/livestream-ecommerce/`
  Key points: livestream commerce needs persistent product context, clear buying paths, and a balance between entertainment/chat and product decision information.

- Nielsen Norman Group, "The Mobile Checkout Experience": `https://www.nngroup.com/articles/mobile-checkout-ux/`
  Key points: mobile checkout is constrained by small screens, interruption, and error-prone input; core order/price clarity and low-friction next steps matter.

- Corefy, "Mobile Checkout UI: Principles & Best Practices": `https://corefy.com/blog/mobile-checkout-ui`
  Key points: persistent order summaries and expandable bottom sheets reduce anxiety and keep price/summary visible without crowding the full flow.

- Invesp, "12 Mobile Checkout Best Practices For e-Commerce Websites": `https://www.invespcro.com/blog/mobile-checkout`
  Key points: reduce mobile distractions, use clear action buttons for next steps, and keep product/order review easy before purchase.

## Infrastructure References That Remain Background

These sources support current architecture reasoning but are not the focus of this phase:

- Redis persistence/AOF docs: `https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/`
- Apache Kafka producer config docs: `https://kafka.apache.org/documentation/#producerconfigs_acks`

## OpenAI/API Implementation Note

When implementing OpenAI-backed features, use current official OpenAI developer docs for exact API request shapes, model choices, image input, streaming, and structured output schema. These details are time-sensitive and should be verified during implementation rather than frozen in this design pack.
