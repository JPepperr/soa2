package pdf

import (
	"fmt"
	"io"
	"os"

	"github.com/signintech/gopdf"

	"mafia/stats/lib/storage"
)

func GeneratePDF(userData storage.UserInfo, imageDate []byte) ([]byte, error) {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	pdf.AddPage()
	err := pdf.AddTTFFont("consolas", "./lib/pdf/consolas.ttf")
	if err != nil {
		return make([]byte, 0), err
	}

	f, err := os.CreateTemp("", "image")
	if err != nil {
		return make([]byte, 0), err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(imageDate); err != nil {
		return make([]byte, 0), err
	}
	if err := f.Close(); err != nil {
		return make([]byte, 0), err
	}

	pdf.Image(f.Name(), 200, 50, nil)

	err = pdf.SetFont("consolas", "", 14)
	if err != nil {
		return make([]byte, 0), err
	}

	pdf.SetXY(200, 200)
	pdf.Cell(nil, fmt.Sprintf("Nickname: %s", userData.Nickname))

	pdf.SetXY(200, 225)
	pdf.Cell(nil, fmt.Sprintf("Sex: %s", userData.Sex))

	pdf.SetXY(200, 250)
	pdf.Cell(nil, fmt.Sprintf("Email: %s", userData.Email))

	pdf.SetXY(200, 275)
	pdf.Cell(nil, fmt.Sprintf("Games count: %d", userData.GamesCnt))

	pdf.SetXY(200, 300)
	pdf.Cell(nil, fmt.Sprintf("Wins: %d", userData.Wins))

	pdf.SetXY(200, 325)
	pdf.Cell(nil, fmt.Sprintf("Loses: %d", userData.Loses))

	pdf.SetXY(200, 350)
	pdf.Cell(nil, fmt.Sprintf("Time in game (nanosec): %d", userData.Duration))

	f, err = os.CreateTemp("", "stat")
	if err != nil {
		return make([]byte, 0), err
	}
	defer os.Remove(f.Name())
	err = pdf.WritePdf(f.Name())
	if err != nil {
		return make([]byte, 0), err
	}

	return io.ReadAll(f)

}
