const sharp = require('sharp');
const fs = require('fs');
const path = require('path');

const svgPath = path.join(__dirname, 'static/icons/icon.svg');
const sizes = [72, 96, 128, 144, 152, 192, 384, 512];

async function generateIcons() {
  try {
    // Read the SVG
    const svgBuffer = fs.readFileSync(svgPath);

    // Generate each size
    for (const size of sizes) {
      console.log(`Generating ${size}x${size} icon...`);

      await sharp(svgBuffer)
        .resize(size, size)
        .png()
        .toFile(path.join(__dirname, `static/icons/icon-${size}x${size}.png`));

      console.log(`✓ Created icon-${size}x${size}.png`);
    }

    // Generate maskable icon (with safe zone)
    console.log('Generating maskable icon...');
    await sharp(svgBuffer)
      .resize(512, 512)
      .png()
      .toFile(path.join(__dirname, 'static/icons/maskable-icon-512x512.png'));

    console.log('✓ Created maskable-icon-512x512.png');
    console.log('All icons generated successfully!');

  } catch (error) {
    console.error('Error generating icons:', error);
    process.exit(1);
  }
}

generateIcons();
