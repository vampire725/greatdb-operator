package encrypt_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/extensions/table"

	. "greatdb.com/greatdb-operator/internal/utils/crypto"

	. "github.com/onsi/gomega"
)

var _ = Describe("greatdbcluster", func() {

	table.DescribeTable("EncryptBase64", func(data string) {
		_, err := EncryptBase64(data)
		fmt.Println(err)
		Expect(err).To(BeNil())

	},
		table.Entry("eg1", "123456"),
		table.Entry("eg2", "greatdb"),
		table.Entry("eg3", "ddd@3Ass"),
	)

	table.DescribeTable("DecryptBase64", func(data string) {
		bs1, _ := DecryptBase64(data)
		res, _ := EncryptBase64(bs1)

		Expect(res).To(Equal(res))

	},
		table.Entry("eg4", "123456"),
		table.Entry("eg5", "greatdb"),
		table.Entry("eg6", "ddd@3Ass"),
	)

})
